package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckURLAlive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := CheckURL(context.Background(), server.URL, 5*time.Second, 0, "test-agent", nil, 0)

	if !result.Alive {
		t.Errorf("expected alive, got %v (status=%d, err=%s)", result.Alive, result.Status, result.Err)
	}
	if result.Status != http.StatusOK {
		t.Errorf("expected status 200, got %d", result.Status)
	}
	if result.URL != server.URL {
		t.Errorf("expected URL %s, got %s", server.URL, result.URL)
	}
}

func TestCheckURLDead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	result := CheckURL(context.Background(), server.URL, 5*time.Second, 0, "test-agent", nil, 0)

	if result.Alive {
		t.Errorf("expected dead, got alive (status=%d)", result.Status)
	}
	if result.Status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", result.Status)
	}
}

func TestCheckURLRedirectChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/target")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := CheckURL(context.Background(), server.URL+"/redirect", 5*time.Second, 0, "test-agent", nil, 0)

	if !result.Alive {
		t.Errorf("expected alive after redirect, got dead")
	}
	if result.Status != http.StatusOK {
		t.Errorf("expected final status 200, got %d", result.Status)
	}
	if len(result.RedirectChain) == 0 {
		t.Fatal("expected redirect chain, got empty")
	}
	if result.RedirectChain[0].Status != http.StatusFound {
		t.Errorf("expected first chain entry status 302, got %d", result.RedirectChain[0].Status)
	}
	if result.RedirectChain[0].URL != server.URL+"/redirect" {
		t.Errorf("expected first chain entry URL %s/redirect, got %s", server.URL, result.RedirectChain[0].URL)
	}
}

func TestCheckURLRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := CheckURL(context.Background(), server.URL, 5*time.Second, 2, "test-agent", nil, 0)

	if !result.Alive {
		t.Errorf("expected alive after retry, got dead")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
}

func TestCheckURLRetryExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	result := CheckURL(context.Background(), server.URL, 5*time.Second, 2, "test-agent", nil, 0)

	if result.Alive {
		t.Errorf("expected dead after exhausting retries, got alive")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
	if result.Status != http.StatusBadGateway {
		t.Errorf("expected final status 502, got %d", result.Status)
	}
}

func TestCheckURLContextCancel(t *testing.T) {
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := CheckURL(ctx, slowServer.URL, 5*time.Second, 0, "test-agent", nil, 0)

	if result.Err == "" {
		t.Error("expected error from cancelled context, got none")
	}
	if result.Alive {
		t.Error("expected dead from cancelled context, got alive")
	}
}

func TestCheckURLCache(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cache := NewCache()

	r1 := CheckURL(context.Background(), server.URL, 5*time.Second, 0, "test-agent", cache, time.Minute)
	r2 := CheckURL(context.Background(), server.URL, 5*time.Second, 0, "test-agent", cache, time.Minute)

	if requests != 1 {
		t.Errorf("expected 1 HTTP request (second served from cache), got %d", requests)
	}
	if r1.Status != r2.Status || r1.Alive != r2.Alive {
		t.Errorf("cache returned different result: %+v vs %+v", r1, r2)
	}
}

func TestCheckURLCacheExpiry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cache := NewCache()

	_ = CheckURL(context.Background(), server.URL, 5*time.Second, 0, "test-agent", cache, -time.Minute) // already expired
	_ = CheckURL(context.Background(), server.URL, 5*time.Second, 0, "test-agent", cache, time.Minute)

	if requests != 2 {
		t.Errorf("expected 2 HTTP requests (first entry expired), got %d", requests)
	}
}

func TestCheckURLErrorStatus(t *testing.T) {
	// Connection refused: use a server that's immediately closed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	result := CheckURL(context.Background(), serverURL, 2*time.Second, 0, "test-agent", nil, 0)

	if result.Alive {
		t.Error("expected dead for connection refused, got alive")
	}
	if result.Status != 0 {
		t.Errorf("expected status 0 for non-HTTP error, got %d", result.Status)
	}
	if result.Err == "" {
		t.Error("expected error message for connection refused, got empty")
	}
}
