package checker

import (
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
