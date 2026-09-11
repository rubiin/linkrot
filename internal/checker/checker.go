package checker

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"linkrot/internal/model"
)

// maxRedirects is the maximum number of redirect hops to follow.
const maxRedirects = 10

// CheckResult is the result of checking a single URL.
type CheckResult struct {
	URL           string
	Status        int
	Alive         bool
	RedirectChain []model.RedirectStep
	Err           string
}

// Cache is an in-memory cache of check results keyed by URL.
type Cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	result  CheckResult
	expires time.Time
}

// NewCache creates an empty check-result cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]cacheEntry)}
}

// get returns the cached result for url if present and not expired.
func (c *Cache) get(rawURL string) (CheckResult, bool) {
	if c == nil {
		return CheckResult{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[rawURL]
	if !ok || time.Now().After(entry.expires) {
		return CheckResult{}, false
	}
	return entry.result, true
}

// put stores the result for url until the TTL elapses. Results with errors
// are not cached so transient failures are retried on the next run.
func (c *Cache) put(rawURL string, result CheckResult, ttl time.Duration) {
	if c == nil || ttl <= 0 || result.Err != "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[rawURL] = cacheEntry{
		result:  result,
		expires: time.Now().Add(ttl),
	}
}

// CheckURL checks a single URL by issuing an HTTP GET, following redirects up
// to maxRedirects, retrying on 502/503/504 up to retryCount times with
// linear backoff. When cache is non-nil, a fresh cached result is returned
// and successful results are stored with the given TTL.
func CheckURL(ctx context.Context, rawURL string, timeout time.Duration, retryCount int, userAgent string, cache *Cache, cacheTTL time.Duration) CheckResult {
	if result, ok := cache.get(rawURL); ok {
		return result
	}

	result := checkOnce(ctx, rawURL, timeout, userAgent)

	for i := 0; i < retryCount && isRetryable(result); i++ {
		backoff := time.Duration(i+1) * 200 * time.Millisecond
		select {
		case <-ctx.Done():
			result.Err = ctx.Err().Error()
			cache.put(rawURL, result, 0)
			return result
		case <-time.After(backoff):
			result = checkOnce(ctx, rawURL, timeout, userAgent)
		}
	}

	cache.put(rawURL, result, cacheTTL)
	return result
}

// isRetryable reports whether a failed check should be retried: a 502, 503,
// or 504 response, or a transport error (result.Err != "" with no status).
func isRetryable(result CheckResult) bool {
	if result.Err != "" && result.Status == 0 {
		return true
	}
	return result.Status == http.StatusBadGateway ||
		result.Status == http.StatusServiceUnavailable ||
		result.Status == http.StatusGatewayTimeout
}

// checkOnce performs a single check attempt: an HTTP GET following redirects,
// recording each hop in the redirect chain.
func checkOnce(ctx context.Context, rawURL string, timeout time.Duration, userAgent string) CheckResult {
	chain := []model.RedirectStep{}
	currentURL := rawURL

	client := &http.Client{
		Timeout: timeout,
		// Do not let the client follow redirects: the loop below follows them
		// manually so each hop can be recorded in the chain.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for hop := 0; ; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return CheckResult{URL: rawURL, Err: err.Error()}
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "*/*")

		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return CheckResult{URL: rawURL, Err: ctx.Err().Error()}
			}
			return CheckResult{URL: rawURL, Status: 0, Err: err.Error()}
		}

		// Drain the body so the connection can be reused.
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 300 && resp.StatusCode < 400 && hop < maxRedirects {
			location := resp.Header.Get("Location")
			if location != "" {
				next, err := resp.Location()
				if err == nil {
					chain = append(chain, model.RedirectStep{
						URL:    currentURL,
						Status: resp.StatusCode,
					})
					currentURL = next.String()
					continue
				}
			}
		}

		// Not a redirect (or redirect could not be followed): this is final.
		return CheckResult{
			URL:           rawURL,
			Status:        resp.StatusCode,
			Alive:         resp.StatusCode >= 200 && resp.StatusCode < 400,
			RedirectChain: chain,
		}
	}
}
