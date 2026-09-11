package checker

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"linkrot/internal/extract"
	"linkrot/internal/model"
)

type CheckConfig struct {
	Root                string
	Threads             int
	Timeout             time.Duration
	CacheTTL            time.Duration
	Retry               int
	UserAgent           string
	AllowFileExtensions []string
	IgnoreHosts         []string
}

// CheckAll extracts the URLs from files, checks each one with a worker pool,
// and returns the results sorted dead links first.
func CheckAll(ctx context.Context, cfg CheckConfig, files []string) []model.LinkCheck {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		root = cfg.Root
	}
	baseURL := "file://" + root

	urlSources := make(map[string][]string)
	var allURLs []string

	for _, file := range files {
		if !allowedFileExtension(file, cfg.AllowFileExtensions) {
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(file)), ".")
		if ext == "" {
			ext = "txt"
		}

		for _, u := range extract.ExtractURLs(content, ext, baseURL) {
			if _, seen := urlSources[u]; !seen {
				allURLs = append(allURLs, u)
			}
			urlSources[u] = append(urlSources[u], file)
		}
	}

	var urlsToCheck []string
	for _, u := range allURLs {
		if !hostIsIgnored(u, cfg.IgnoreHosts) {
			urlsToCheck = append(urlsToCheck, u)
		}
	}

	threads := cfg.Threads
	if threads < 1 {
		threads = 1
	}

	cache := NewCache()
	results := make([]model.LinkCheck, 0, len(urlsToCheck))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, threads)

	for _, u := range urlsToCheck {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(rawURL string) {
			defer wg.Done()
			defer func() { <-sem }()

			r := CheckURL(ctx, rawURL, cfg.Timeout, cfg.Retry, cfg.UserAgent, cache, cfg.CacheTTL)

			mu.Lock()
			results = append(results, model.LinkCheck{
				URL:           r.URL,
				Status:        r.Status,
				Alive:         r.Alive,
				RedirectChain: r.RedirectChain,
				Err:           r.Err,
				SourceFile:    strings.Join(urlSources[rawURL], ", "),
			})
			mu.Unlock()
		}(u)
	}

	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		if results[i].Alive != results[j].Alive {
			return !results[i].Alive
		}
		return results[i].URL < results[j].URL
	})

	return results
}
