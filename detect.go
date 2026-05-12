package main

import (
	"strings"
)

// isSubstack returns true when the homepage HTML or URL indicates the site is
// hosted on Substack. We check several markers because Substack publications
// can run on custom domains and the markers differ across template versions.
func isSubstack(html, target string) bool {
	if html == "" {
		return false
	}
	lower := strings.ToLower(html)

	// Strong signals first.
	markers := []string{
		`name="generator" content="substack"`,
		`content="substack" name="generator"`,
		"substackcdn.com",
		"substack-post-media",
		`"is_substack":true`,
		`"isSubstack":true`,
		`window._preloads`, // Substack's preload payload
		"bundle-hashes-",   // substack asset hash naming
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}

	// Host-based fallback for .substack.com domains.
	if strings.Contains(strings.ToLower(target), ".substack.com") {
		return true
	}
	return false
}
