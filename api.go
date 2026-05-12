package main

import (
	"encoding/json"
	"strings"
)

// parsePostsAPI consumes the response from /api/v1/archive or /api/v1/posts.
// The endpoint returns a JSON array of post objects, but the field set has
// evolved over time; we decode loosely and pick the most stable fields.
func parsePostsAPI(body []byte, base string) []Post {
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		// Some publications return {"posts":[...]}; try the wrapped form.
		var wrap struct {
			Posts []map[string]any `json:"posts"`
		}
		if err2 := json.Unmarshal(body, &wrap); err2 != nil {
			return nil
		}
		raw = wrap.Posts
	}
	out := make([]Post, 0, len(raw))
	for _, item := range raw {
		p := Post{
			Title:            asString(item["title"]),
			Slug:             asString(item["slug"]),
			Published:        asString(item["post_date"]),
			URL:              asString(item["canonical_url"]),
			IsSubscriberOnly: asBool(item["audience"]) || asString(item["audience"]) == "only_paid",
			Description:      asString(item["description"]),
		}
		if p.Published == "" {
			p.Published = asString(item["publication_date"])
		}
		if p.URL == "" && p.Slug != "" {
			p.URL = strings.TrimRight(base, "/") + "/p/" + p.Slug
		}
		// Subscriber-only check based on audience field as string.
		if aud, ok := item["audience"].(string); ok {
			p.IsSubscriberOnly = aud == "only_paid" || aud == "founding"
		}
		if p.Title == "" && p.URL == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// parsePodcastAPI consumes /api/v1/podcast/episodes. Returns nil if there
// are no podcast episodes (publication isn't a podcast).
func parsePodcastAPI(body []byte, base string) *Podcast {
	var raw []map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	pod := &Podcast{}
	for _, item := range raw {
		ep := PodcastEpisode{
			Title:     asString(item["title"]),
			Published: asString(item["post_date"]),
			Duration:  int(asFloat(item["audio_items_duration_seconds"])),
		}
		if slug := asString(item["slug"]); slug != "" {
			ep.URL = strings.TrimRight(base, "/") + "/p/" + slug
		}
		if ep.Title == "" {
			continue
		}
		pod.Episodes = append(pod.Episodes, ep)
		if len(pod.Episodes) >= 12 {
			break
		}
	}
	if len(pod.Episodes) == 0 {
		return nil
	}
	return pod
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func asFloat(v any) float64 {
	if v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}
