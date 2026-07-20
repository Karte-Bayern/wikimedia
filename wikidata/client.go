package wikidata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

var (
	itemIDPattern     = regexp.MustCompile(`^Q[1-9][0-9]*$`)
	propertyIDPattern = regexp.MustCompile(`^P[1-9][0-9]*$`)
)

// Client reads Wikidata entities through wbgetentities.
type Client struct {
	api       *mediawiki.Client
	languages []string
	batchSize int
}

// NewClient creates a Wikidata client.
func NewClient(options ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	api := cfg.mediawikiClient
	if api == nil {
		var err error
		api, err = mediawiki.NewClient(cfg.endpoint,
			mediawiki.WithHTTPClient(cfg.httpClient), mediawiki.WithUserAgent(cfg.userAgent),
			mediawiki.WithMaxLag(cfg.maxLag), mediawiki.WithMaxResponseBytes(cfg.maxResponseBytes),
			mediawiki.WithRetryPolicy(cfg.retry), mediawiki.WithCache(cfg.cache, cfg.cacheTTL),
		)
		if err != nil {
			return nil, err
		}
	}
	languages := normalizeLanguages(cfg.languages)
	if len(languages) == 0 {
		languages = []string{"en"}
	}
	if cfg.batchSize <= 0 || cfg.batchSize > 50 {
		cfg.batchSize = 50
	}
	return &Client{api: api, languages: languages, batchSize: cfg.batchSize}, nil
}

// ValidItemID reports whether value is a non-zero Wikidata Q ID.
func ValidItemID(value string) bool { return itemIDPattern.MatchString(strings.TrimSpace(value)) }

// ValidPropertyID reports whether value is a non-zero Wikidata property ID.
func ValidPropertyID(value string) bool {
	return propertyIDPattern.MatchString(strings.TrimSpace(value))
}

// GetEntity fetches one item.
func (c *Client) GetEntity(ctx context.Context, id string) (*Entity, error) {
	id = strings.TrimSpace(id)
	entities, err := c.GetEntities(ctx, []string{id})
	if err != nil {
		return nil, err
	}
	entity, ok := entities[id]
	if !ok || entity.Missing {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	copy := entity
	return &copy, nil
}

// GetEntityBySitelink resolves a page title on a Wikimedia site (for example,
// "dewiki") to its linked Wikidata item.
func (c *Client) GetEntityBySitelink(ctx context.Context, site, title string) (*Entity, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("wikidata: nil client")
	}
	site, title = strings.TrimSpace(site), strings.TrimSpace(title)
	if site == "" || title == "" {
		return nil, fmt.Errorf("%w: empty site or title", ErrNotFound)
	}
	parameters := url.Values{
		"action": {"wbgetentities"}, "sites": {site}, "titles": {title},
		"props":     {"labels|descriptions|aliases|claims|sitelinks"},
		"languages": {strings.Join(c.languages, "|")}, "languagefallback": {"1"}, "redirects": {"yes"},
	}
	entities, err := c.queryEntities(ctx, parameters)
	if err != nil {
		return nil, err
	}
	for _, entity := range entities {
		if !entity.Missing {
			copy := entity
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("%w: %s:%s", ErrNotFound, site, title)
}

// GetEntityByStatement resolves an exact external-ID statement to one item.
// It uses WikibaseCirrusSearch's haswbstatement query and returns its first
// matching item.
func (c *Client) GetEntityByStatement(ctx context.Context, property, value string) (*Entity, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("wikidata: nil client")
	}
	property, value = strings.ToUpper(strings.TrimSpace(property)), strings.TrimSpace(value)
	if !ValidPropertyID(property) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidProperty, property)
	}
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return nil, fmt.Errorf("%w: empty statement value", ErrNotFound)
	}
	parameters := url.Values{
		"action": {"query"}, "list": {"search"}, "srnamespace": {"0"}, "srlimit": {"1"},
		"srsearch": {"haswbstatement:" + property + "=" + value},
	}
	var envelope struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if _, err := c.api.Query(ctx, parameters, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Query.Search) == 0 || !ValidItemID(envelope.Query.Search[0].Title) {
		return nil, fmt.Errorf("%w: %s=%s", ErrNotFound, property, value)
	}
	return c.GetEntity(ctx, envelope.Query.Search[0].Title)
}

// GetEntities fetches items in anonymous-safe batches of at most 50 IDs.
func (c *Client) GetEntities(ctx context.Context, ids []string) (map[string]Entity, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("wikidata: nil client")
	}
	clean := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if !ValidItemID(id) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidID, raw)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	result := make(map[string]Entity, len(clean))
	for start := 0; start < len(clean); start += c.batchSize {
		end := start + c.batchSize
		if end > len(clean) {
			end = len(clean)
		}
		batch, err := c.getBatch(ctx, clean[start:end])
		if err != nil {
			return nil, err
		}
		for id, entity := range batch {
			result[id] = entity
		}
	}
	return result, nil
}

func (c *Client) getBatch(ctx context.Context, ids []string) (map[string]Entity, error) {
	parameters := url.Values{
		"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")},
		"props":     {"labels|descriptions|aliases|claims|sitelinks"},
		"languages": {strings.Join(c.languages, "|")}, "languagefallback": {"1"},
		"redirects": {"yes"},
	}
	return c.queryEntities(ctx, parameters)
}

func (c *Client) queryEntities(ctx context.Context, parameters url.Values) (map[string]Entity, error) {
	var envelope struct {
		Entities map[string]json.RawMessage `json:"entities"`
	}
	if _, err := c.api.Query(ctx, parameters, &envelope); err != nil {
		return nil, err
	}
	result := make(map[string]Entity, len(envelope.Entities))
	for key, raw := range envelope.Entities {
		var entity Entity
		if err := json.Unmarshal(raw, &entity); err != nil {
			return nil, fmt.Errorf("wikidata: decode %s: %w", key, err)
		}
		entity.Raw = append(json.RawMessage(nil), raw...)
		if entity.ID == "" {
			entity.ID = key
		}
		result[key] = entity
	}
	return result, nil
}

func normalizeLanguages(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// SortedClaimProperties returns deterministic claim property IDs.
func (e Entity) SortedClaimProperties() []string {
	result := make([]string, 0, len(e.Claims))
	for property := range e.Claims {
		result = append(result, property)
	}
	sort.Strings(result)
	return result
}
