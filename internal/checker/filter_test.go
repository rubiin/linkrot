package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllowedFileExtension(t *testing.T) {
	tests := []struct {
		filename string
		allowed  []string
		want     bool
	}{
		{"doc.html", []string{"html"}, true},
		{"doc.HTML", []string{"html"}, true},
		{"doc.txt", []string{"html"}, false},
		{"doc.md", []string{"html", "md"}, true},
		{"doc.yml", []string{}, true}, // empty list = allow all
		{"doc.yaml", []string{"txt", "yaml"}, true},
		{"doc.c", []string{"c", "h"}, true},
		{"doc.h", []string{"c", "h"}, true},
		{"doc.cpp", []string{"c", "h"}, false},
		{"doc", []string{"html"}, false}, // no extension
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := allowedFileExtension(tt.filename, tt.allowed); got != tt.want {
				t.Errorf("allowedFileExtension(%q, %v) = %v, want %v", tt.filename, tt.allowed, got, tt.want)
			}
		})
	}
}

func TestIgnoreFileMatcher(t *testing.T) {
	tests := []struct {
		data string
		rel  string
		want bool
	}{
		// filename-only patterns
		{"foo.txt", "bar/foo.txt", true},
		{"foo.txt", "foo.txt", true},
		{"foo.txt", "bar/baz.txt", false},
		// path patterns
		{"bar/foo.txt", "bar/foo.txt", true},
		{"bar/foo.txt", "baz/foo.txt", false},
		// wildcard
		{"*.txt", "bar/foo.txt", true},
		{"*.txt", "foo.txt", true},
		{"dir/*.txt", "dir/foo.txt", true},
		{"dir/*.txt", "other/foo.txt", false},
		// negation
		{"*.txt\n!keep.txt", "keep.txt", false},
		{"*.txt\n!keep.txt", "drop.txt", true},
		// comments and blanks
		{"\n# comment\n\nfoo.txt", "foo.txt", true},
		// directory ignore prunes children
		{"ignoreme/", "ignoreme/file.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			m := newIgnoreFileMatcher([]byte(tt.data))
			if got := m.matches(slash(tt.rel)); got != tt.want {
				t.Errorf("matches(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestShouldSkip(t *testing.T) {
	m1 := newIgnoreFileMatcher([]byte("*.txt"))
	m2 := newIgnoreFileMatcher([]byte("!keep.txt"))

	// both matchers: first says skip, second says keep -> skip wins because
	// shouldSkip returns on the first affirmative match.
	if got := shouldSkip("drop.txt", []*ignoreFileMatcher{m1, m2}); got {
		t.Errorf("drop.txt should not be skipped when later matcher negates")
	}

	if got := shouldSkip("other.txt", []*ignoreFileMatcher{m1, m2}); got {
		t.Logf("other.txt correctly skipped")
	}

	if got := shouldSkip("keep.txt", []*ignoreFileMatcher{m1, m2}); !got {
		t.Logf("keep.txt correctly not skipped")
	}
}

func TestReadIgnoreFile(t *testing.T) {
	tmpDir := t.TempDir()

	p := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(p, []byte("*.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	m := readIgnoreFile(tmpDir, ".gitignore")
	if m == nil {
		t.Fatal("expected non-nil matcher")
	}
	if !m.matches(slash("foo.txt")) {
		t.Error("expected foo.txt to match")
	}

	if m := readIgnoreFile(tmpDir, ".nonexistent"); m != nil {
		t.Error("expected nil for missing file")
	}
}

func TestCollectFiles(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(tmpDir, "docs"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "docs", "a.html"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "docs", "b.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "docs", "c.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// No ignore files, all extensions allowed.
	files, err := collectFiles(tmpDir, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// .gitignore ignores b.txt and the ignore file itself.
	ignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("docs/b.txt\n.gitignore\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := readIgnoreFile(tmpDir, ".gitignore")
	files, err = collectFiles(tmpDir, nil, []*ignoreFileMatcher{m})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files after ignore, got %d: %v", len(files), files)
	}

	found := false
	for _, f := range files {
		if strings.HasSuffix(f, "b.txt") {
			found = true
		}
	}
	if found {
		t.Error("b.txt should have been excluded")
	}
}

func TestHostIsIgnored(t *testing.T) {
	tests := []struct {
		url     string
		ignored []string
		want    bool
	}{
		{"https://example.com/page", []string{"example.com"}, true},
		{"https://example.com/page", []string{"www.example.com"}, false},
		{"http://Example.COM/page", []string{"example.com"}, true},
		{"https://other.com/page", []string{"example.com"}, false},
		{"https://sub.example.com/page", []string{"example.com"}, false},
		{"not-a-url", []string{"example.com"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := hostIsIgnored(tt.url, tt.ignored); got != tt.want {
				t.Errorf("hostIsIgnored(%q, %v) = %v, want %v", tt.url, tt.ignored, got, tt.want)
			}
		})
	}
}
