package checker

import (
	"net/url"
	"path"
	"strings"
)

// allowedFileExtension reports whether the file's extension is in the allowed
// list. Entries may be given with or without a leading dot. An empty list
// allows all files.
func allowedFileExtension(filename string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(filename)), ".")
	for _, a := range allowed {
		if ext == strings.TrimPrefix(strings.ToLower(a), ".") {
			return true
		}
	}
	return false
}

// hostIsIgnored reports whether the URL's host matches one of the ignored
// hosts (case-insensitive, exact host match).
func hostIsIgnored(rawURL string, ignored []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return false
	}
	for _, h := range ignored {
		if host == strings.ToLower(strings.TrimSpace(h)) {
			return true
		}
	}
	return false
}
