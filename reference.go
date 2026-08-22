package wikimedia

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/karte-bayern/wikimedia/wikidata"
)

// FetchByReference resolves a Wikidata ID or URL, a Wikipedia page URL, or an
// OpenStreetMap object URL. It also accepts the compact forms "wikidata:Q64",
// "wikipedia:de:Berlin", and "relation/62422" for command-line use.
func (c *Client) FetchByReference(ctx context.Context, reference string, options ...FetchOption) (*Result, error) {
	reference = strings.TrimSpace(reference)
	if wikidata.ValidItemID(reference) {
		return c.Fetch(ctx, reference, options...)
	}
	if strings.HasPrefix(strings.ToLower(reference), "file:") && strings.TrimSpace(reference[len("file:"):]) != "" {
		return c.FetchByCommonsFile(ctx, reference, options...)
	}
	if strings.HasPrefix(strings.ToLower(reference), "category:") && strings.TrimSpace(reference[len("category:"):]) != "" {
		return c.FetchByCommonsCategory(ctx, reference, options...)
	}
	if prefix, value, found := strings.Cut(reference, ":"); found {
		switch strings.ToLower(strings.TrimSpace(prefix)) {
		case "wikidata":
			return c.Fetch(ctx, value, options...)
		case "wikipedia":
			language, title, ok := strings.Cut(value, ":")
			if ok {
				return c.FetchByWikipedia(ctx, language, title, options...)
			}
		}
	}
	if objectType, id, found := strings.Cut(reference, "/"); found {
		switch OSMType(strings.ToLower(strings.TrimSpace(objectType))) {
		case OSMRelation, OSMWay, OSMNode:
			return c.FetchByOSM(ctx, OSMType(strings.ToLower(strings.TrimSpace(objectType))), id, options...)
		}
	}
	parsed, err := url.Parse(reference)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedReference, reference)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "wikidata.org" || host == "www.wikidata.org" {
		if id := wikidataIDFromURL(parsed); id != "" {
			return c.Fetch(ctx, id, options...)
		}
	}
	if title, kind, ok := commonsReferenceFromURL(host, parsed); ok {
		if kind == "file" {
			return c.FetchByCommonsFile(ctx, title, options...)
		}
		return c.FetchByCommonsCategory(ctx, title, options...)
	}
	if language, title, ok := wikipediaReferenceFromURL(host, parsed); ok {
		return c.FetchByWikipedia(ctx, language, title, options...)
	}
	if objectType, id, ok := osmReferenceFromURL(host, parsed); ok {
		return c.FetchByOSM(ctx, objectType, id, options...)
	}
	return nil, fmt.Errorf("%w: %q", ErrUnsupportedReference, reference)
}

func commonsReferenceFromURL(host string, value *url.URL) (string, string, bool) {
	if host != "commons.wikimedia.org" {
		return "", "", false
	}
	path := strings.Trim(value.EscapedPath(), "/")
	if path == "w/index.php" {
		path = "wiki/" + value.Query().Get("title")
	}
	title, found := strings.CutPrefix(path, "wiki/")
	if !found || title == "" || strings.Contains(title, "/") {
		return "", "", false
	}
	decoded, err := url.PathUnescape(title)
	if err != nil {
		return "", "", false
	}
	decoded = strings.ReplaceAll(decoded, "_", " ")
	switch {
	case strings.HasPrefix(strings.ToLower(decoded), "file:"):
		return decoded, "file", true
	case strings.HasPrefix(strings.ToLower(decoded), "category:"):
		return decoded, "category", true
	default:
		return "", "", false
	}
}

func wikidataIDFromURL(value *url.URL) string {
	path := strings.Trim(value.EscapedPath(), "/")
	for _, prefix := range []string{"wiki/", "entity/"} {
		if id, found := strings.CutPrefix(path, prefix); found && !strings.Contains(id, "/") {
			if decoded, err := url.PathUnescape(id); err == nil && wikidata.ValidItemID(decoded) {
				return decoded
			}
		}
	}
	if strings.HasSuffix(path, "w/index.php") {
		if id := value.Query().Get("title"); wikidata.ValidItemID(id) {
			return id
		}
	}
	return ""
}

func wikipediaReferenceFromURL(host string, value *url.URL) (string, string, bool) {
	suffix := ".wikipedia.org"
	if !strings.HasSuffix(host, suffix) {
		return "", "", false
	}
	language := strings.TrimSuffix(host, suffix)
	language = strings.TrimSuffix(language, ".m")
	if !wikipediaLanguageIDPattern.MatchString(language) {
		return "", "", false
	}
	path := strings.Trim(value.EscapedPath(), "/")
	if title, found := strings.CutPrefix(path, "wiki/"); found && title != "" && !strings.Contains(title, "/") {
		decoded, err := url.PathUnescape(title)
		if err == nil {
			return language, strings.ReplaceAll(decoded, "_", " "), true
		}
	}
	if path == "w/index.php" {
		if title := strings.TrimSpace(value.Query().Get("title")); title != "" {
			return language, title, true
		}
	}
	return "", "", false
}

func osmReferenceFromURL(host string, value *url.URL) (OSMType, string, bool) {
	if host != "openstreetmap.org" && host != "www.openstreetmap.org" && host != "osm.org" && host != "www.osm.org" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(value.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	objectType := OSMType(strings.ToLower(parts[0]))
	if objectType != OSMRelation && objectType != OSMWay && objectType != OSMNode {
		return "", "", false
	}
	id, err := url.PathUnescape(parts[1])
	if err != nil || id == "" {
		return "", "", false
	}
	return objectType, id, true
}
