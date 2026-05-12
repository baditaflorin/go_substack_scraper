# go_substack_scraper

Extracts public metadata from a Substack publication. Given a target URL the
service detects whether the site is hosted on Substack, then fans out to a
few public Substack API endpoints plus the homepage HTML and `/subscribe`
page to produce a single JSON payload covering:

- Publication identity (name, tagline, founders, logo).
- Recent posts (title, slug, URL, publish date, subscriber-only flag).
- Podcast episodes when present.
- Pricing tiers from `/subscribe`.
- Subscriber count when visible on the homepage.
- Cross-recommended Substack publications.

## Running

```
go run .
```

The service listens on `PORT` (default `8236`) and exposes:

- `GET /health` - liveness probe.
- `GET /t/{token}/?target=<substack-url>` - scrape target.
- `GET /go_substack_scraper?target=<substack-url>` - same handler, alt path.

## Constraints

- SSRF guard rejects loopback, private, link-local, CGNAT, multicast IPs.
- Hard total budget of 12s, per-request 5s, body capped at 4MB.
- `errgroup.SetLimit(3)` - max 3 concurrent endpoint fetches.
- Branded User-Agent.

## Output

See top-level `Response` struct in `handler.go`.
