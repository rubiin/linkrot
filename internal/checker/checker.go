package checker

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"linkrot/internal/model"
)

const maxRedirects = 10

type CheckResult struct {
	URL           string
	Status        int
	Alive         bool
	RedirectChain []model.RedirectStep
	Err           string
}

type Cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	result  CheckResult
	expires time.Time
}

func NewCache() *Cache {
	return &Cache{entries: make(map[string]cacheEntry)}
}

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

// put skips failed results so transient failures get retried on the next run.
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

// CheckURL fetches rawURL, following redirects up to maxRedirects and retrying
// on 502/503/504 or transport errors. Results are cached for cacheTTL.
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
			return result
		case <-time.After(backoff):
			result = checkOnce(ctx, rawURL, timeout, userAgent)
		}
	}

	cache.put(rawURL, result, cacheTTL)
	return result
}

func isRetryable(result CheckResult) bool {
	if result.Err != "" && result.Status == 0 {
		return true
	}
	return result.Status == http.StatusBadGateway ||
		result.Status == http.StatusServiceUnavailable ||
		result.Status == http.StatusGatewayTimeout
}

// checkOnce issues one GET, walking redirects itself so every hop lands in the chain.
func checkOnce(ctx context.Context, rawURL string, timeout time.Duration, userAgent string) CheckResult {
	chain := []model.RedirectStep{}
	currentURL := rawURL

	client := &http.Client{
		Timeout: timeout,
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
			if next, err := resp.Location(); err == nil {
				chain = append(chain, model.RedirectStep{
					URL:    currentURL,
					Status: resp.StatusCode,
				})
				currentURL = next.String()
				continue
			}
		}

		return CheckResult{
			URL:           rawURL,
			Status:        resp.StatusCode,
			Alive:         resp.StatusCode >= 200 && resp.StatusCode < 400,
			RedirectChain: chain,
		}
	}
}
