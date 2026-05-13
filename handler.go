package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/baditaflorin/go-common/safehttp"
	"golang.org/x/sync/errgroup"
)

const (
	version      = "1.0.1"
	userAgent    = "go_substack_scraper/" + version + " (+https://github.com/baditaflorin/go_substack_scraper)"
	fetchTimeout = 5 * time.Second
	totalBudget  = 12 * time.Second
	maxBodyBytes = 4 * 1024 * 1024
	maxRedirects = 4
	maxPosts     = 24
	apiWorkers   = 3
)

// Response is the JSON shape returned to callers.
type Response struct {
	Tool            string       `json:"tool"`
	Version         string       `json:"version"`
	Target          string       `json:"target"`
	IsSubstack      bool         `json:"is_substack"`
	Publication     *Publication `json:"publication,omitempty"`
	Subscribers     *Subscribers `json:"subscribers,omitempty"`
	Posts           []Post       `json:"posts"`
	Podcast         *Podcast     `json:"podcast,omitempty"`
	Pricing         *Pricing     `json:"pricing,omitempty"`
	Recommendations []string     `json:"recommendations"`
	TotalPosts      int          `json:"total_posts"`
	Truncated       bool         `json:"truncated"`
	Error           string       `json:"error,omitempty"`
}

type Publication struct {
	Name     string   `json:"name,omitempty"`
	URL      string   `json:"url,omitempty"`
	Tagline  string   `json:"tagline,omitempty"`
	Founders []string `json:"founders,omitempty"`
	HeroText string   `json:"hero_text,omitempty"`
	Logo     string   `json:"logo,omitempty"`
}

type Subscribers struct {
	Display string `json:"display,omitempty"`
	Parsed  int64  `json:"parsed,omitempty"`
	Source  string `json:"source,omitempty"`
}

type Post struct {
	Title            string `json:"title,omitempty"`
	Slug             string `json:"slug,omitempty"`
	Published        string `json:"published,omitempty"`
	URL              string `json:"url,omitempty"`
	IsSubscriberOnly bool   `json:"is_subscriber_only"`
	Description      string `json:"description,omitempty"`
}

type Podcast struct {
	Title    string           `json:"title,omitempty"`
	Episodes []PodcastEpisode `json:"episodes,omitempty"`
}

type PodcastEpisode struct {
	Title     string `json:"title,omitempty"`
	URL       string `json:"url,omitempty"`
	Published string `json:"published,omitempty"`
	Duration  int    `json:"duration_seconds,omitempty"`
}

type Pricing struct {
	FreeTier bool     `json:"free_tier"`
	Paid     *PaidTier `json:"paid_tier,omitempty"`
}

type PaidTier struct {
	MonthlyUSD int `json:"monthly_usd,omitempty"`
	AnnualUSD  int `json:"annual_usd,omitempty"`
	FounderUSD int `json:"founder_usd,omitempty"`
}

var httpClient = func() *http.Client {
	c := safehttp.NewClient()
	c.Timeout = fetchTimeout
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		_, err := safehttp.CheckURL(req.Context(), req.URL.String())
		return err
	}
	return c
}()

func writeJSON(w http.ResponseWriter, status int, resp Response) {
	resp.Tool = "go_substack_scraper"
	resp.Version = version
	if resp.Posts == nil {
		resp.Posts = []Post{}
	}
	if resp.Recommendations == nil {
		resp.Recommendations = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// Handler is the HTTP entry point. It SSRF-guards the target, detects whether
// the site is a Substack publication, then fans out to a few public Substack
// API endpoints plus the homepage HTML under a hard time budget.
func Handler(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		target = r.URL.Query().Get("url")
	}
	if target == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: "Missing 'target' or 'url' query parameter"})
		return
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	u, err := url.Parse(target)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Target: target, Error: "Invalid URL: " + err.Error()})
		return
	}
	if _, err := safehttp.CheckURL(r.Context(), u.String()); err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Target: target, Error: err.Error()})
		return
	}

	// Normalize: keep scheme + host, drop path/query for the publication root.
	root := &url.URL{Scheme: u.Scheme, Host: u.Host}
	rootStr := root.String()

	ctx, cancel := context.WithTimeout(r.Context(), totalBudget)
	defer cancel()

	// Step 1: fetch homepage to detect Substack hosting.
	home, homeStatus, err := fetch(ctx, rootStr)
	if err != nil {
		writeJSON(w, http.StatusOK, Response{
			Target: rootStr,
			Error:  fmt.Sprintf("homepage fetch failed: %v", err),
		})
		return
	}
	_ = homeStatus

	if !isSubstack(home, rootStr) {
		writeJSON(w, http.StatusOK, Response{
			Target:     rootStr,
			IsSubstack: false,
		})
		return
	}

	resp := Response{
		Target:     rootStr,
		IsSubstack: true,
	}

	// Step 2: fan out to API endpoints + subscribe page in parallel.
	type apiResult struct {
		kind string
		body []byte
		ok   bool
	}
	endpoints := []struct {
		kind, path string
	}{
		{"posts", "/api/v1/archive?sort=new&limit=12"},
		{"podcast", "/api/v1/podcast/episodes"},
		{"subscribe", "/subscribe"},
	}
	results := make([]apiResult, len(endpoints))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(apiWorkers)
	for i, ep := range endpoints {
		i, ep := i, ep
		g.Go(func() error {
			body, _, err := fetchBytes(gctx, rootStr+ep.path)
			if err != nil {
				results[i] = apiResult{kind: ep.kind, ok: false}
				return nil
			}
			results[i] = apiResult{kind: ep.kind, body: body, ok: true}
			return nil
		})
	}
	_ = g.Wait()

	// Step 3: extract from homepage HTML.
	resp.Publication = extractPublication(home, rootStr)
	resp.Subscribers = extractSubscribers(home)
	resp.Recommendations = extractRecommendations(home, u.Host)

	// Step 4: process API results.
	for _, res := range results {
		if !res.ok {
			continue
		}
		switch res.kind {
		case "posts":
			resp.Posts = parsePostsAPI(res.body, rootStr)
		case "podcast":
			resp.Podcast = parsePodcastAPI(res.body, rootStr)
		case "subscribe":
			resp.Pricing = parsePricing(string(res.body))
		}
	}

	// Fallback: extract posts from homepage HTML if API returned nothing.
	if len(resp.Posts) == 0 {
		resp.Posts = extractPostsFromHTML(home, rootStr)
	}

	// Truncation.
	resp.TotalPosts = len(resp.Posts)
	if len(resp.Posts) > maxPosts {
		resp.Posts = resp.Posts[:maxPosts]
		resp.Truncated = true
	}

	writeJSON(w, http.StatusOK, resp)
}



func fetch(ctx context.Context, target string) (string, int, error) {
	body, status, err := fetchBytes(ctx, target)
	if err != nil {
		return "", status, err
	}
	return string(body), status, nil
}

func fetchBytes(ctx context.Context, target string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/json,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read failed: %w", err)
	}
	if len(data) > maxBodyBytes {
		data = data[:maxBodyBytes]
	}
	return data, resp.StatusCode, nil
}
