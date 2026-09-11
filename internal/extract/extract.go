package extract

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var (
	markdownLinkRe = regexp.MustCompile(`[!]?\[[^\]]*\]\((https?://[^\s)]+)\)`)
	bareURLRe      = regexp.MustCompile(`https?://[^\s)>\]"']+`)
)

// ExtractURLs dispatches on the file extension; unknown extensions fall back
// to bare URL extraction.
func ExtractURLs(content []byte, ext string, baseURL string) []string {
	switch strings.ToLower(ext) {
	case "html":
		return ExtractHTML(content, baseURL)
	case "md", "mkd", "mkdown", "markdown":
		return ExtractMarkdown(content, baseURL)
	default:
		return ExtractBareURLs(content, baseURL)
	}
}

// ExtractHTML pulls hrefs and srcs, resolving relative links against baseURL.
func ExtractHTML(content []byte, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	doc, err := html.Parse(strings.NewReader(string(content)))
	if err != nil {
		return nil
	}

	var links []string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			var attrName string
			switch n.Data {
			case "a", "link":
				attrName = "href"
			case "img", "script", "source", "iframe":
				attrName = "src"
			}
			if attrName != "" {
				for _, attr := range n.Attr {
					if attr.Key == attrName {
						if abs := resolveToAbs(attr.Val, base); abs != "" {
							links = append(links, abs)
						}
						break
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return links
}

// ExtractMarkdown collects [text](url) links plus bare URLs in the prose.
func ExtractMarkdown(content []byte, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}

	var results []string
	seen := make(map[string]bool)
	text := string(content)

	for _, match := range markdownLinkRe.FindAllStringSubmatch(text, -1) {
		if abs := resolveToAbs(match[1], base); abs != "" && !seen[abs] {
			results = append(results, abs)
			seen[abs] = true
		}
	}

	for _, match := range bareURLRe.FindAllString(text, -1) {
		cleaned := strings.TrimRight(match, "),.;:]>\"'")
		if abs := resolveToAbs(cleaned, base); abs != "" && !seen[abs] {
			results = append(results, abs)
			seen[abs] = true
		}
	}

	return results
}

// ExtractBareURLs finds plain http(s) URLs, e.g. in txt or yaml files.
func ExtractBareURLs(content []byte, baseURL string) []string {
	var results []string
	seen := make(map[string]bool)
	text := string(content)

	for _, match := range bareURLRe.FindAllString(text, -1) {
		cleaned := strings.TrimRight(match, "),.;:]>\"'")
		if abs := resolveToAbs(cleaned, nil); abs != "" && !seen[abs] {
			results = append(results, abs)
			seen[abs] = true
		}
	}

	return results
}

// resolveToAbs skips non-http schemes and returns "" for anything that
// doesn't end up as an absolute http(s) URL.
func resolveToAbs(raw string, base *url.URL) string {
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "file:") ||
		strings.HasPrefix(lower, "#") {
		return ""
	}

	rel, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	abs := rel
	if !rel.IsAbs() && base != nil {
		abs = base.ResolveReference(rel)
	}

	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}

	return abs.String()
}
