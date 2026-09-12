package checker

import (
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
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

// ignoreFileMatcher matches Unix globs from an ignore file (one pattern per
// line). Blank lines and lines starting with # are ignored. Patterns without
// a slash are matched against the file name only; patterns containing a slash
// are matched against the relative path from the root.
type ignoreFileMatcher struct {
	patterns []string
}

// newIgnoreFileMatcher parses the contents of an ignore file.
func newIgnoreFileMatcher(data []byte) *ignoreFileMatcher {
	p := ignoreFileMatcher{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p.patterns = append(p.patterns, line)
	}
	return &p
}

// matches reports whether name (relative to root, slash-separated) matches any
// pattern. Unmatched patterns are skipped; a leading "!" negates a match.
func (m *ignoreFileMatcher) matches(rel string) bool {
	base := path.Base(rel)
	// Walk once; the last matching pattern wins (Git-style). We keep it
	// simple and treat any match as a decision, with later negations flipping.
	matched := false
	for _, pat := range m.patterns {
		neg := false
		raw := pat
		if strings.HasPrefix(raw, "!") {
			neg = true
			raw = raw[1:]
		}
		if matchedPattern(raw, rel, base) {
			matched = true
			if neg {
				matched = false
			}
		}
	}
	return matched
}

func matchedPattern(pat, rel, base string) bool {
	dirPattern := strings.HasSuffix(pat, "/")
	if dirPattern {
		pat = strings.TrimRight(pat, "/")
	}
	if strings.Contains(pat, "/") || dirPattern {
		if dirPattern {
			return strings.HasPrefix(rel, pat) || rel == pat
		}
		return filepathMatch(pat, rel)
	}
	return filepathMatch("*/"+pat, rel) || filepathMatch(pat, base)
}

// filepathMatch is a small glob matcher tuned to the patterns Git ignore files
// normally use: *, ?, ** and [a-z]. It is deliberately simple and does not
// handle every edge case, but covers the vast majority of real-world ignore
// patterns.
func filepathMatch(pattern, name string) bool {
	pattern = filepathClean(pattern)
	name = filepathClean(name)
	return globMatch(pattern, name)
}

func filepathClean(p string) string {
	// Collapse repeated separators and remove trailing slash for matching.
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.TrimRight(p, "/")
}

func globMatch(pattern, name string) bool {
	pi, ni := 0, 0
	var starPI, starNI = -1, -1

	for ni < len(name) {
		if pi < len(pattern) && (pattern[pi] == name[ni] || pattern[pi] == '?') {
			pi++
			ni++
			continue
		}
		if pi < len(pattern) && pattern[pi] == '*' {
			starPI = pi
			starNI = ni
			pi++
			continue
		}
		if pi < len(pattern) && pattern[pi] == '[' {
			close := strings.IndexByte(pattern[pi:], ']')
			if close == -1 {
				return false
			}
			rangeEnd := pi + close
			rng := pattern[pi : rangeEnd+1]
			pi = rangeEnd + 1
			if bisectMatch(rng, name[ni]) {
				ni++
				continue
			}
			if starPI != -1 {
				pi = starPI + 1
				ni = starNI + 1
				starNI++
				continue
			}
			return false
		}
		if starPI != -1 {
			pi = starPI + 1
			starNI++
			ni = starNI
			continue
		}
		return false
	}

	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

func bisectMatch(rng string, c byte) bool {
	negate := rng[0] == '!'
	if negate {
		rng = rng[1:]
	}
	match := false
	for i := 0; i < len(rng); i++ {
		switch rng[i] {
		case ']':
			if i == 0 {
				match = match || c == ']'
				break
			}
			return match
		case '-':
			if i == 0 || i == len(rng)-1 || rng[i+1] == ']' {
				match = match || c == '-'
			} else {
				match = match || (c >= rng[i-1] && c <= rng[i+1])
				return match
			}
		default:
			match = match || c == rng[i]
		}
	}
	if negate {
		match = !match
	}
	return match
}

// shouldSkip reports whether rel (relative to root, slash-separated) should
// be skipped. Each matcher is evaluated in order (each file can override the
// previous), and the final decision is returned.
func shouldSkip(rel string, matchers []*ignoreFileMatcher) bool {
	decision := false
	for _, m := range matchers {
		if m != nil {
			decision = m.matches(rel)
		}
	}
	return decision
}

// readIgnoreFile reads and parses an ignore file. Missing or unreadable files
// are silently ignored.
func readIgnoreFile(root, name string) *ignoreFileMatcher {
	p := filepath.Join(root, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	return newIgnoreFileMatcher(data)
}

// collectFiles walks root and returns every regular file that is not skipped
// by the supplied ignore matchers. Directories that are ignored are pruned.
func collectFiles(root string, allowed []string, matchers []*ignoreFileMatcher) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	// Ensure root ends with a separator so TrimPrefix produces clean relative paths.
	root = root + string(filepath.Separator)

	var out []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := strings.TrimPrefix(p, root)
		if rel == "" {
			return nil
		}
		rel = slash(rel)
		if d.IsDir() {
			if shouldSkip(rel, matchers) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if shouldSkip(rel, matchers) {
				return nil
			}
			if allowedFileExtension(p, allowed) {
				out = append(out, p)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func slash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
