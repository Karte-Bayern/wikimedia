package wikidata

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const defaultSearchLimit = 10

// SearchResult is one Wikidata item matched by a full-text search.
type SearchResult struct {
	ID          string   `json:"id"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	URL         string   `json:"url,omitempty"`
	Match       string   `json:"match,omitempty"`
}

// SearchOption configures one item search.
type SearchOption func(*searchConfig)

type searchConfig struct {
	language string
	limit    int
}

// SearchLanguage sets the language used for item labels and descriptions.
func SearchLanguage(value string) SearchOption {
	return func(c *searchConfig) { c.language = value }
}

// SearchLimit limits results to at most 50 Wikidata items. A non-positive
// value uses the default of 10.
func SearchLimit(value int) SearchOption { return func(c *searchConfig) { c.limit = value } }

// SearchItems performs a Wikidata full-text search for items. Use this for
// labels, names, and aliases; use SPARQL for structured graph queries.
func (c *Client) SearchItems(ctx context.Context, query string, options ...SearchOption) ([]SearchResult, error) {
	if c == nil || c.api == nil {
		return nil, fmt.Errorf("wikidata: nil client")
	}
	query = strings.TrimSpace(query)
	if query == "" || strings.ContainsAny(query, "\r\n") {
		return nil, fmt.Errorf("%w: %q", ErrInvalidSearch, query)
	}
	cfg := searchConfig{language: c.languages[0], limit: defaultSearchLimit}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	cfg.language = strings.ToLower(strings.TrimSpace(cfg.language))
	if cfg.language == "" {
		cfg.language = c.languages[0]
	}
	if cfg.limit <= 0 {
		cfg.limit = defaultSearchLimit
	}
	if cfg.limit > 50 {
		cfg.limit = 50
	}
	parameters := url.Values{
		"action": {"wbsearchentities"}, "search": {query}, "language": {cfg.language},
		"type": {"item"}, "limit": {strconv.Itoa(cfg.limit)},
	}
	var envelope struct {
		Search []struct {
			ID          string   `json:"id"`
			Label       string   `json:"label"`
			Description string   `json:"description"`
			Aliases     []string `json:"aliases"`
			ConceptURI  string   `json:"concepturi"`
			Match       *struct {
				Text string `json:"text"`
			} `json:"match"`
		} `json:"search"`
	}
	if _, err := c.api.Query(ctx, parameters, &envelope); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(envelope.Search))
	for _, item := range envelope.Search {
		if !ValidItemID(item.ID) {
			continue
		}
		result := SearchResult{ID: item.ID, Label: item.Label, Description: item.Description, Aliases: append([]string(nil), item.Aliases...), URL: item.ConceptURI}
		if item.Match != nil {
			result.Match = item.Match.Text
		}
		results = append(results, result)
	}
	return results, nil
}
