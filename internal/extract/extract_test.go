package extract

import (
	"testing"
)

func TestExtractHTML(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body>
<a href="/page">relative</a>
<a href="https://example.com/absolute">absolute</a>
<a href="http://example.com/bare">bare http</a>
<img src="https://cdn.example.com/img.png">
<link href="/style.css">
<script src="https://cdn.example.com/app.js"></script>
<a href="mailto:test@example.com">email</a>
<a href="#anchor">anchor</a>
<a href="file:///local/path">file</a>
</body>
</html>`

	baseURL := "https://example.com/docs/"
	results := ExtractURLs([]byte(html), "html", baseURL)

	expected := []string{
		"https://example.com/page",
		"https://example.com/absolute",
		"http://example.com/bare",
		"https://cdn.example.com/img.png",
		"https://example.com/style.css",
		"https://cdn.example.com/app.js",
	}

	if len(results) != len(expected) {
		t.Fatalf("got %d URLs, want %d: %v", len(results), len(expected), results)
	}

	for _, e := range expected {
		found := false
		for _, r := range results {
			if r == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected URL %s; got %v", e, results)
		}
	}
}

func TestExtractMarkdown(t *testing.T) {
	md := `# Title

[link text](https://example.com/page)
![alt text](http://images.example.com/img.png)
[bare](https://example.com/bare)

https://example.com/bare-link
http://example.com/another-bare

[relative](/relative/page)

[mailto](mailto:test@example.com)
`

	baseURL := "https://example.com/docs/"
	results := ExtractURLs([]byte(md), "md", baseURL)

	expected := []string{
		"https://example.com/page",
		"http://images.example.com/img.png",
		"https://example.com/bare",
		"https://example.com/bare-link",
		"http://example.com/another-bare",
	}

	if len(results) != len(expected) {
		t.Fatalf("got %d URLs, want %d: %v", len(results), len(expected), results)
	}

	for _, e := range expected {
		found := false
		for _, r := range results {
			if r == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected URL %s; got %v", e, results)
		}
	}
}

func TestExtractText(t *testing.T) {
	txt := `Check out https://example.com/page and http://other.example.org/foo
Also https://example.com/page (duplicate)`
	baseURL := "https://example.com/docs/"
	results := ExtractURLs([]byte(txt), "txt", baseURL)

	expected := []string{
		"https://example.com/page",
		"http://other.example.org/foo",
	}

	if len(results) != len(expected) {
		t.Fatalf("got %d URLs, want %d: %v", len(results), len(expected), results)
	}

	seen := make(map[string]bool)
	for _, r := range results {
		seen[r] = true
	}
	for _, e := range expected {
		if !seen[e] {
			t.Errorf("missing expected URL %s; got %v", e, results)
		}
	}
}
