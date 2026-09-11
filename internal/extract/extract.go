package extract

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var (
	// markdownLinkRe matches [text](url) and ![alt](url)
	markdownLinkRe = regexp.MustCompile(`[!]?\[[^\]]*\]\((https?://[^\s)]+)\)`)
	// bareURLRe matches standalone http(s):// URLs in text
	bareURLRe = regexp.MustCompile(`https?://[^\s)>\]"']+`)
)

// ExtractURLs extracts HTTP(S) URLs from content based on file extension.
// Supported extensions: html, md, mkd, mkdown, markdown, txt, yaml, yml, c, h, cpp, hpp.
// For unknown extensions, only bare URL extraction is used.
func ExtractURLs(content []byte, ext string, baseURL string) []string {
	switch strings.ToLower(ext) {
	case "html":
		return ExtractHTML(content, baseURL)
	case "md", "mkd", "mkdown", "markdown":
		return ExtractMarkdown(content, baseURL)
	case "txt", "yaml", "yml", "c", "h", "cpp", "hpp":
		return ExtractBareURLs(content, baseURL)
	default:
		return ExtractBareURLs(content, baseURL)
	}
}

// ExtractHTML extracts HTTP(S) URLs from HTML content, resolving relative URLs
// against baseURL. Collects href attributes from <a> and <link>, src attributes
// from <img>, <script>, <source>, and <iframe>.
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

// ExtractMarkdown extracts HTTP(S) URLs from Markdown content: [text](url),
// ![alt](url), and bare http(s):// URLs.
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

	// Bare URLs in addition to markdown links
	for _, match := range bareURLRe.FindAllString(text, -1) {
		cleaned := strings.TrimRight(match, "),.;:]>\"'")
		if abs := resolveToAbs(cleaned, base); abs != "" && !seen[abs] {
			results = append(results, abs)
			seen[abs] = true
		}
	}

	return results
}

// ExtractBareURLs extracts only bare http(s):// URLs from content.
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

// resolveToAbs parses raw and resolves it against base (when base is non-nil
// and raw is relative), returning the absolute URL string if it is http(s).
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
