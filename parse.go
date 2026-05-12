package main

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	reTitle       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reMetaName    = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:site_name["'][^>]+content=["']([^"']+)["']`)
	reMetaDesc    = regexp.MustCompile(`(?is)<meta[^>]+(?:name|property)=["'](?:description|og:description)["'][^>]+content=["']([^"']+)["']`)
	reMetaImage   = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	reSubsCount   = regexp.MustCompile(`(?i)([\d,]+(?:\.\d+)?\s*[KMkm]?)\s*(?:subscribers|readers)`)
	rePostLink    = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']*/p/[^"'#?]+)["'][^>]*>(.*?)</a>`)
	reFounders    = regexp.MustCompile(`(?is)"name"\s*:\s*"([^"]{2,80})"\s*,\s*"is_founder"\s*:\s*true`)
	rePriceMonth  = regexp.MustCompile(`\$\s*(\d{1,3}(?:\.\d{1,2})?)\s*(?:/|\s+per\s+)\s*month`)
	rePriceYear   = regexp.MustCompile(`\$\s*(\d{1,4}(?:\.\d{1,2})?)\s*(?:/|\s+per\s+)\s*year`)
	rePriceFound  = regexp.MustCompile(`(?i)founding\s*member[^$]{0,80}\$\s*(\d{1,4})`)
	reHostExtract = regexp.MustCompile(`https?://([a-zA-Z0-9.\-]+\.substack\.com)`)
)

// extractPublication mines the homepage HTML for the publication identity.
func extractPublication(html_ string, base string) *Publication {
	p := &Publication{URL: base}
	if m := reMetaName.FindStringSubmatch(html_); len(m) > 1 {
		p.Name = decode(m[1])
	}
	if p.Name == "" {
		if m := reTitle.FindStringSubmatch(html_); len(m) > 1 {
			p.Name = strings.TrimSpace(decode(m[1]))
		}
	}
	if m := reMetaDesc.FindStringSubmatch(html_); len(m) > 1 {
		p.Tagline = decode(m[1])
	}
	if m := reMetaImage.FindStringSubmatch(html_); len(m) > 1 {
		p.Logo = m[1]
	}
	for _, m := range reFounders.FindAllStringSubmatch(html_, -1) {
		if len(m) > 1 {
			p.Founders = append(p.Founders, decode(m[1]))
		}
	}
	if len(p.Founders) == 0 {
		p.Founders = []string{}
	}
	return p
}

// extractSubscribers does a best-effort regex sweep for "N subscribers".
// Substack only sometimes exposes this on the public homepage; absence is
// not an error.
func extractSubscribers(html_ string) *Subscribers {
	m := reSubsCount.FindStringSubmatch(html_)
	if len(m) < 2 {
		return nil
	}
	raw := strings.TrimSpace(m[1])
	parsed := parseHumanInt(raw)
	if parsed == 0 {
		return nil
	}
	return &Subscribers{
		Display: raw + " subscribers",
		Parsed:  parsed,
		Source:  "homepage_text",
	}
}

// parseHumanInt interprets "12K", "1.5M", "12,345" as int64.
func parseHumanInt(s string) int64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0
	}
	mult := int64(1)
	switch last := s[len(s)-1]; last {
	case 'K', 'k':
		mult = 1_000
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1_000_000
		s = s[:len(s)-1]
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * float64(mult))
}

// extractPostsFromHTML is the fallback when the public posts API is blocked.
// It scrapes <a href=".../p/SLUG"> elements from the homepage.
func extractPostsFromHTML(html_ string, base string) []Post {
	seen := map[string]bool{}
	out := []Post{}
	for _, m := range rePostLink.FindAllStringSubmatch(html_, -1) {
		href := m[1]
		if !strings.HasPrefix(href, "http") {
			href = strings.TrimRight(base, "/") + href
		}
		if seen[href] {
			continue
		}
		seen[href] = true
		slug := postSlug(href)
		title := strings.TrimSpace(decode(stripTags(m[2])))
		if title == "" {
			continue
		}
		out = append(out, Post{Title: title, Slug: slug, URL: href})
		if len(out) >= 24 {
			break
		}
	}
	return out
}

func postSlug(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// extractRecommendations finds other Substack publications recommended on
// the homepage (Substack's network of "recommendations" widget). We restrict
// to other hosts to avoid self-references.
func extractRecommendations(html_ string, selfHost string) []string {
	seen := map[string]bool{strings.ToLower(selfHost): true}
	out := []string{}
	for _, m := range reHostExtract.FindAllStringSubmatch(html_, -1) {
		host := strings.ToLower(m[1])
		if seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
		if len(out) >= 12 {
			break
		}
	}
	return out
}

// parsePricing reads the /subscribe page HTML for $X/month, $Y/year, founding
// tier prices. Substack renders prices server-side as plain text in the
// subscribe page, so this regex sweep is sufficient.
func parsePricing(htmlBody string) *Pricing {
	pricing := &Pricing{FreeTier: strings.Contains(htmlBody, "Free") || strings.Contains(htmlBody, "free")}
	paid := &PaidTier{}
	hasPaid := false
	if m := rePriceMonth.FindStringSubmatch(htmlBody); len(m) > 1 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			paid.MonthlyUSD = int(v)
			hasPaid = true
		}
	}
	if m := rePriceYear.FindStringSubmatch(htmlBody); len(m) > 1 {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			paid.AnnualUSD = int(v)
			hasPaid = true
		}
	}
	if m := rePriceFound.FindStringSubmatch(htmlBody); len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			paid.FounderUSD = v
			hasPaid = true
		}
	}
	if hasPaid {
		pricing.Paid = paid
	}
	return pricing
}

func decode(s string) string {
	return html.UnescapeString(strings.TrimSpace(s))
}

var reTag = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string {
	return reTag.ReplaceAllString(s, " ")
}
