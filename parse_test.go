package main

import (
	"testing"
)

func TestParseHumanInt(t *testing.T) {
	cases := map[string]int64{
		"15K":     15000,
		"1.5M":    1500000,
		"12,345":  12345,
		"500":     500,
		"":        0,
		"3k":      3000,
		"2.2m":    2200000,
	}
	for in, want := range cases {
		if got := parseHumanInt(in); got != want {
			t.Errorf("parseHumanInt(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIsSubstack(t *testing.T) {
	cases := map[string]bool{
		`<html><meta name="generator" content="Substack"></html>`:                       true,
		`<html><script src="https://substackcdn.com/bundle.js"></script></html>`:        true,
		`<html><title>Random site</title></html>`:                                       false,
		``: false,
	}
	for in, want := range cases {
		if got := isSubstack(in, "https://example.com"); got != want {
			t.Errorf("isSubstack on %q = %v, want %v", in[:min(40, len(in))], got, want)
		}
	}
	if !isSubstack("<html></html>", "https://yann.substack.com/") {
		t.Errorf("isSubstack should be true for substack.com host")
	}
}

func TestParsePricing(t *testing.T) {
	html := `Become a paid subscriber for $8/month or $80/year. Founding member $150.`
	p := parsePricing(html)
	if p.Paid == nil {
		t.Fatal("expected paid tier")
	}
	if p.Paid.MonthlyUSD != 8 || p.Paid.AnnualUSD != 80 || p.Paid.FounderUSD != 150 {
		t.Errorf("got %+v", p.Paid)
	}
}

func TestExtractRecommendations(t *testing.T) {
	html := `<a href="https://foo.substack.com/x">x</a><a href="https://bar.substack.com">y</a><a href="https://foo.substack.com">dup</a>`
	got := extractRecommendations(html, "self.substack.com")
	if len(got) != 2 {
		t.Errorf("expected 2 recs, got %d: %v", len(got), got)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
