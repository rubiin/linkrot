package checker

import (
	"net/url"
	"path"
	"strings"
)

// allowedFileExtension treats an empty list as "allow everything" and
// tolerates a leading dot on either side.
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
