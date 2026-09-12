package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	htmlFile := filepath.Join(tmpDir, "test.html")
	err := os.WriteFile(htmlFile, []byte(`<a href="`+server.URL+`/page">link</a>`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:      tmpDir,
		Threads:   2,
		Timeout:   5 * time.Second,
		CacheTTL:  0,
		Retry:     0,
		UserAgent: "test-agent",
	}

	results := CheckAll(context.Background(), cfg, []string{htmlFile})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), results)
	}

	if !results[0].Alive {
		t.Errorf("expected alive, got dead (status=%d, err=%s)", results[0].Status, results[0].Err)
	}
	if results[0].SourceFile != htmlFile {
		t.Errorf("expected source file %s, got %s", htmlFile, results[0].SourceFile)
	}
}

func TestCheckAllIgnoreHosts(t *testing.T) {
	tmpDir := t.TempDir()
	htmlFile := filepath.Join(tmpDir, "test.html")
	err := os.WriteFile(htmlFile, []byte(`<a href="https://www.example.com/page">link</a>`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:        tmpDir,
		Threads:     2,
		Timeout:     5 * time.Second,
		CacheTTL:    0,
		Retry:       0,
		UserAgent:   "test-agent",
		IgnoreHosts: []string{"www.example.com"},
	}

	results := CheckAll(context.Background(), cfg, []string{htmlFile})

	if len(results) != 0 {
		t.Errorf("expected 0 results (all ignored), got %d: %v", len(results), results)
	}
}

func TestCheckAllFileExtensionFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	htmlFile := filepath.Join(tmpDir, "test.html")
	err := os.WriteFile(htmlFile, []byte(`<a href="`+server.URL+`/page">link</a>`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	txtFile := filepath.Join(tmpDir, "test.txt")
	err = os.WriteFile(txtFile, []byte(server.URL+`/txt-page`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:                tmpDir,
		Threads:             2,
		Timeout:             5 * time.Second,
		CacheTTL:            0,
		Retry:               0,
		UserAgent:           "test-agent",
		AllowFileExtensions: []string{"html"},
	}

	results := CheckAll(context.Background(), cfg, []string{htmlFile, txtFile})

	if len(results) != 1 {
		t.Errorf("expected 1 result (html only), got %d: %v", len(results), results)
	}
}

func TestCheckAllDeduplicatesURLs(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "one.html")
	file2 := filepath.Join(tmpDir, "two.html")
	same := []byte(`<a href="` + server.URL + `/page">link</a>`)
	if err := os.WriteFile(file1, same, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, same, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:      tmpDir,
		Threads:   2,
		Timeout:   5 * time.Second,
		CacheTTL:  0,
		Retry:     0,
		UserAgent: "test-agent",
	}

	results := CheckAll(context.Background(), cfg, []string{file1, file2})

	if len(results) != 1 {
		t.Errorf("expected 1 result (URL deduplicated across files), got %d: %v", len(results), results)
	}
	if requests.Load() != 1 {
		t.Errorf("expected 1 HTTP request, got %d", requests.Load())
	}
}

func TestCheckAllRespectsConcurrency(t *testing.T) {
	var current atomic.Int64
	var peak atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := current.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		current.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	var files []string
	for i := 0; i < 10; i++ {
		f := filepath.Join(tmpDir, "f"+string(rune('a'+i))+".html")
		if err := os.WriteFile(f, []byte(`<a href="`+server.URL+`/p`+string(rune('a'+i))+`">x</a>`), 0644); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}

	cfg := CheckConfig{
		Root:      tmpDir,
		Threads:   3,
		Timeout:   5 * time.Second,
		CacheTTL:  0,
		Retry:     0,
		UserAgent: "test-agent",
	}

	results := CheckAll(context.Background(), cfg, files)

	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
	if peak.Load() > 3 {
		t.Errorf("expected concurrency peak <= 3, got %d", peak.Load())
	}
}

func TestCheckAllDirectoryExpansion(t *testing.T) {
	tmpDir := t.TempDir()

	docDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docDir, 0755); err != nil {
		t.Fatal(err)
	}

	htmlFile := filepath.Join(docDir, "page.html")
	if err := os.WriteFile(htmlFile, []byte(`<a href="http://example.com/a">a</a>`), 0644); err != nil {
		t.Fatal(err)
	}

	txtFile := filepath.Join(docDir, "notes.txt")
	if err := os.WriteFile(txtFile, []byte("http://example.com/b"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:      tmpDir,
		Threads:   1,
		Timeout:   5 * time.Second,
		CacheTTL:  0,
		Retry:     0,
		UserAgent: "test-agent",
	}

	results := CheckAll(context.Background(), cfg, []string{docDir})

	if len(results) != 2 {
		t.Fatalf("expected 2 results from directory walk, got %d: %v", len(results), results)
	}
}

func TestCheckAllIgnoredByGitignore(t *testing.T) {
	tmpDir := t.TempDir()

	docDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docDir, 0755); err != nil {
		t.Fatal(err)
	}

	ignoredFile := filepath.Join(docDir, "ignored.html")
	if err := os.WriteFile(ignoredFile, []byte(`<a href="http://example.com/ignored">x</a>`), 0644); err != nil {
		t.Fatal(err)
	}

	keptFile := filepath.Join(docDir, "kept.html")
	if err := os.WriteFile(keptFile, []byte(`<a href="http://example.com/kept">x</a>`), 0644); err != nil {
		t.Fatal(err)
	}

	gitignore := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("docs/ignored.html\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:                tmpDir,
		Threads:             1,
		Timeout:             5 * time.Second,
		CacheTTL:            0,
		Retry:               0,
		UserAgent:           "test-agent",
		IgnoreFiles:         []string{".gitignore"},
		AllowFileExtensions: []string{"html"},
	}

	results := CheckAll(context.Background(), cfg, []string{docDir})

	if len(results) != 1 {
		t.Fatalf("expected 1 result (ignored file skipped), got %d: %v", len(results), results)
	}
	if results[0].URL != "http://example.com/kept" {
		t.Errorf("expected kept URL, got %s", results[0].URL)
	}
}

func TestCheckAllDirectoryAndExplicitFiles(t *testing.T) {
	tmpDir := t.TempDir()

	docDir := filepath.Join(tmpDir, "docs")
	if err := os.MkdirAll(docDir, 0755); err != nil {
		t.Fatal(err)
	}

	dirFile := filepath.Join(docDir, "from-dir.html")
	if err := os.WriteFile(dirFile, []byte(`<a href="http://example.com/dir">x</a>`), 0644); err != nil {
		t.Fatal(err)
	}

	standalone := filepath.Join(tmpDir, "standalone.html")
	if err := os.WriteFile(standalone, []byte(`<a href="http://example.com/standalone">x</a>`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:                tmpDir,
		Threads:             1,
		Timeout:             5 * time.Second,
		CacheTTL:            0,
		Retry:               0,
		UserAgent:           "test-agent",
		AllowFileExtensions: []string{"html"},
	}

	results := CheckAll(context.Background(), cfg, []string{docDir, standalone})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}
}

func TestCheckAllMixedAliveDead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dead" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "mix.html")
	content := `<a href="` + server.URL + `/alive">a</a><a href="` + server.URL + `/dead">d</a>`
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := CheckConfig{
		Root:      tmpDir,
		Threads:   2,
		Timeout:   5 * time.Second,
		CacheTTL:  0,
		Retry:     0,
		UserAgent: "test-agent",
	}

	results := CheckAll(context.Background(), cfg, []string{f})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}
	// Dead first
	if results[0].Alive {
		t.Errorf("expected first result dead, got alive ordering: %v", results)
	}
	if results[1].URL != server.URL+"/alive" {
		t.Errorf("expected second result to be the alive URL, got %s", results[1].URL)
	}
}
