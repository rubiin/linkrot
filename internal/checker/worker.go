package checker

import (
	"context"
	"io/fs"
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
	IgnoreFiles         []string
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isDirectory reports whether path is a directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isUnder reports whether candidate is under root (or equal to it).
func isUnder(candidate, root string) bool {
	candidate = filepath.Clean(candidate) + string(filepath.Separator)
	root = filepath.Clean(root) + string(filepath.Separator)
	return strings.HasPrefix(candidate, root)
}

// fileCount tracks the number of files scanned in the current CheckAll call.
var fileCount int

// scannedDir tracks whether the input was a directory (or multiple files).
var scannedDir bool

// FileCount returns the number of files scanned in the most recent CheckAll call.
func FileCount() int {
	return fileCount
}

// ScannedDirectory returns whether the most recent CheckAll call scanned a directory.
func ScannedDirectory() bool {
	return scannedDir
}

// CheckAll extracts the URLs from files, checks each one with a worker pool,
// and returns the results sorted dead links first.
func CheckAll(ctx context.Context, cfg CheckConfig, files []string) []model.LinkCheck {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		root = cfg.Root
	}
	// Ensure root ends with a separator for clean relative paths.
	root = root + string(filepath.Separator)
	baseURL := "file://" + root

	// Build ignore matchers from any configured ignore files.
	var matchers []*ignoreFileMatcher
	for _, name := range cfg.IgnoreFiles {
		if m := readIgnoreFile(root, name); m != nil {
			matchers = append(matchers, m)
		}
	}

	// Determine the set of files to process. Explicit files are used directly;
	// directories are expanded by walking from root and keeping only files that
	// live under one of the specified entries.
	var walkEntries []string
	for _, entry := range files {
		abs, err := filepath.Abs(entry)
		if err != nil {
			continue
		}
		walkEntries = append(walkEntries, abs)
	}

	var work []string
	if len(walkEntries) == 0 {
		// No valid entries after stat; nothing to do.
		return nil
	}

	if len(walkEntries) == 1 && fileExists(walkEntries[0]) && !isDirectory(walkEntries[0]) {
		// Single explicit file: use it directly.
		work = []string{walkEntries[0]}
	} else {
		// Walk from root and collect files under any of the specified entries.
		visited := make(map[string]bool)
		walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel := strings.TrimPrefix(p, root)
			rel = slash(rel)
			if shouldSkip(rel, matchers) {
				return nil
			}
			if !allowedFileExtension(p, cfg.AllowFileExtensions) {
				return nil
			}
			for _, entry := range walkEntries {
				if isUnder(p, entry) {
					if !visited[p] {
						visited[p] = true
						work = append(work, p)
					}
					break
				}
			}
			return nil
		})
		if walkErr != nil {
			return nil
		}
	}
	fileCount = len(work)
	// Consider it a directory scan if we walked (multiple entries or single directory)
	// or if more than one file was processed.
	scannedDir = len(walkEntries) > 1 || (len(walkEntries) == 1 && isDirectory(walkEntries[0]))

	urlSources := make(map[string][]string)
	var allURLs []string

	for _, file := range work {
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
