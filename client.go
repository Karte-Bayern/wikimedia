package wikimedia

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/karte-bayern/wikimedia/commons"
	"github.com/karte-bayern/wikimedia/mediawiki"
	"github.com/karte-bayern/wikimedia/wikidata"
	"github.com/karte-bayern/wikimedia/wikipedia"
)

// Client aggregates Wikidata, Commons, and Wikipedia.
type Client struct {
	wikidata  *wikidata.Client
	sparql    *wikidata.SPARQLClient
	commons   *commons.Client
	wikipedia *wikipedia.Client
	languages []string
	now       func() time.Time
}

// New creates an aggregate client. A descriptive application User-Agent is mandatory.
func New(options ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if err := mediawiki.ValidateUserAgent(cfg.userAgent); err != nil {
		return nil, err
	}
	languages := normalizeLanguages(cfg.languages)
	userAgent := strings.TrimSpace(cfg.userAgent) + " karte-bayern-wikimedia/" + Version
	wd, err := wikidata.NewClient(
		wikidata.WithEndpoint(cfg.wikidataEndpoint), wikidata.WithHTTPClient(cfg.httpClient),
		wikidata.WithUserAgent(userAgent), wikidata.WithMaxLag(cfg.maxLag),
		wikidata.WithMaxResponseBytes(cfg.maxResponseBytes), wikidata.WithRetryPolicy(cfg.retry),
		wikidata.WithCache(cfg.cache, cfg.cacheTTLs.Wikidata), wikidata.WithLanguages(languages...),
	)
	if err != nil {
		return nil, fmt.Errorf("wikimedia: create Wikidata client: %w", err)
	}
	sp, err := wikidata.NewSPARQLClient(
		wikidata.WithSPARQLEndpoint(cfg.sparqlEndpoint), wikidata.WithSPARQLHTTPClient(cfg.httpClient),
		wikidata.WithSPARQLUserAgent(userAgent), wikidata.WithSPARQLMaxResponseBytes(cfg.maxResponseBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("wikimedia: create SPARQL client: %w", err)
	}
	cm, err := commons.NewClient(
		commons.WithEndpoint(cfg.commonsEndpoint), commons.WithHTTPClient(cfg.httpClient),
		commons.WithUserAgent(userAgent), commons.WithMaxLag(cfg.maxLag),
		commons.WithMaxResponseBytes(cfg.maxResponseBytes), commons.WithRetryPolicy(cfg.retry),
		commons.WithCache(cfg.cache, cfg.cacheTTLs.Commons), commons.WithLanguage(languages[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("wikimedia: create Commons client: %w", err)
	}
	wp, err := wikipedia.NewClient(
		wikipedia.WithHTTPClient(cfg.httpClient), wikipedia.WithUserAgent(userAgent),
		wikipedia.WithMaxLag(cfg.maxLag), wikipedia.WithMaxResponseBytes(cfg.maxResponseBytes),
		wikipedia.WithRetryPolicy(cfg.retry), wikipedia.WithCache(cfg.cache, cfg.cacheTTLs.Wikipedia),
		wikipedia.WithEndpointTemplate(cfg.wikipediaEndpointTemplate),
	)
	if err != nil {
		return nil, fmt.Errorf("wikimedia: create Wikipedia client: %w", err)
	}
	return &Client{wikidata: wd, sparql: sp, commons: cm, wikipedia: wp, languages: languages, now: time.Now}, nil
}

// Wikidata returns the low-level Wikidata client used by this aggregate client.
func (c *Client) Wikidata() *wikidata.Client {
	if c == nil {
		return nil
	}
	return c.wikidata
}

// SPARQL returns the bounded Wikidata Query Service client used by this aggregate client.
func (c *Client) SPARQL() *wikidata.SPARQLClient {
	if c == nil {
		return nil
	}
	return c.sparql
}

// Commons returns the low-level Commons client used by this aggregate client.
func (c *Client) Commons() *commons.Client {
	if c == nil {
		return nil
	}
	return c.commons
}

// Wikipedia returns the low-level Wikipedia client used by this aggregate client.
func (c *Client) Wikipedia() *wikipedia.Client {
	if c == nil {
		return nil
	}
	return c.wikipedia
}

var (
	osmIDPattern               = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)
	wikipediaLanguageIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)
)

// FetchByWikipedia resolves a Wikipedia page title to its Wikidata item and
// returns the same normalized result as Fetch. Language is a Wikipedia
// language code such as "de" or "en".
func (c *Client) FetchByWikipedia(ctx context.Context, language, title string, options ...FetchOption) (*Result, error) {
	if c == nil || c.wikidata == nil {
		return nil, errors.New("wikimedia: nil client")
	}
	language = strings.ToLower(strings.TrimSpace(language))
	if !wikipediaLanguageIDPattern.MatchString(language) {
		return nil, fmt.Errorf("wikimedia: invalid Wikipedia language %q", language)
	}
	entity, err := c.wikidata.GetEntityBySitelink(ctx, language+"wiki", title)
	if err != nil {
		return nil, err
	}
	return c.fetchEntity(ctx, entity, newFetchConfig(options))
}

// FetchByOSM resolves an OpenStreetMap relation, way, or node ID through its
// corresponding Wikidata external-ID statement, then returns a normalized
// result. OSM IDs are not globally unique across object types.
func (c *Client) FetchByOSM(ctx context.Context, objectType OSMType, id string, options ...FetchOption) (*Result, error) {
	if c == nil || c.wikidata == nil {
		return nil, errors.New("wikimedia: nil client")
	}
	id = strings.TrimSpace(id)
	if !osmIDPattern.MatchString(id) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidOSMID, id)
	}
	property := ""
	switch objectType {
	case OSMRelation:
		property = "P402"
	case OSMWay:
		property = "P10689"
	case OSMNode:
		property = "P11693"
	default:
		return nil, fmt.Errorf("%w: type %q", ErrInvalidOSMID, objectType)
	}
	entity, err := c.wikidata.GetEntityByStatement(ctx, property, id)
	if err != nil {
		return nil, err
	}
	return c.fetchEntity(ctx, entity, newFetchConfig(options))
}

// Fetch enriches one Wikidata item.
func (c *Client) Fetch(ctx context.Context, id string, options ...FetchOption) (*Result, error) {
	if c == nil || c.wikidata == nil {
		return nil, errors.New("wikimedia: nil client")
	}
	entity, err := c.wikidata.GetEntity(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	return c.fetchEntity(ctx, entity, newFetchConfig(options))
}

func newFetchConfig(options []FetchOption) fetchConfig {
	cfg := defaultFetchConfig()
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	normalizeFetchConfig(&cfg)
	return cfg
}

func (c *Client) fetchEntity(ctx context.Context, entity *wikidata.Entity, cfg fetchConfig) (*Result, error) {
	result := normalizeEntity(entity, c.languages, cfg)
	result.FetchedAt = c.now().UTC()

	if cfg.mediaLimit > 0 && cfg.directMedia {
		c.resolveDirectMedia(ctx, result, entity, cfg)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if cfg.mediaLimit > 0 && cfg.categories {
		c.resolveCategoryMedia(ctx, result, cfg)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	result.Media = rankAndLimitMedia(result.Media, cfg.mediaLimit)
	if cfg.wikipedia && cfg.articleLimit > 0 {
		c.resolveWikipedia(ctx, result, entity, cfg)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// FetchMedia returns only the media portion of Fetch.
func (c *Client) FetchMedia(ctx context.Context, id string, options ...FetchOption) ([]Media, error) {
	result, err := c.Fetch(ctx, id, options...)
	if err != nil {
		return nil, err
	}
	return result.Media, nil
}

func normalizeFetchConfig(cfg *fetchConfig) {
	if cfg.mediaLimit < 0 {
		cfg.mediaLimit = 20
	}
	if cfg.thumbnailWidth <= 0 {
		cfg.thumbnailWidth = 1200
	}
	if cfg.articleLimit < 0 {
		cfg.articleLimit = 1
	}
	if cfg.categoryPageLimit < 0 {
		cfg.categoryPageLimit = 3
	}
	valid := cfg.mediaProperties[:0]
	for _, property := range cfg.mediaProperties {
		property.ID = strings.ToUpper(strings.TrimSpace(property.ID))
		if property.ID == "" {
			continue
		}
		if property.Kind == "" {
			property.Kind = MediaKindOther
		}
		valid = append(valid, property)
	}
	cfg.mediaProperties = valid
}

func (c *Client) resolveDirectMedia(ctx context.Context, result *Result, entity *wikidata.Entity, cfg fetchConfig) {
	type reference struct {
		title  string
		source MediaSource
	}
	var refs []reference
	uniqueTitles := make([]string, 0)
	seenTitle := map[string]struct{}{}
	for _, property := range cfg.mediaProperties {
		for _, claim := range entity.Claims[property.ID] {
			if claim.Rank == "deprecated" && !cfg.deprecated {
				continue
			}
			value, ok := claim.MainSnak.StringValue()
			if !ok || strings.TrimSpace(value) == "" {
				continue
			}
			title := strings.TrimSpace(value)
			refs = append(refs, reference{title: title, source: MediaSource{
				Service: ServiceWikidata, Kind: "claim", Property: property.ID, ClaimID: claim.ID,
				Rank: claim.Rank, Value: title, Direct: true, MediaKind: property.Kind, BaseScore: property.BaseScore,
			}})
			key := mediaTitleKey(title)
			if _, exists := seenTitle[key]; !exists {
				seenTitle[key] = struct{}{}
				uniqueTitles = append(uniqueTitles, title)
			}
		}
	}
	if len(uniqueTitles) == 0 {
		return
	}
	files, err := c.commons.GetFiles(ctx, uniqueTitles, commons.FileThumbnailWidth(cfg.thumbnailWidth))
	if err != nil {
		result.Warnings = append(result.Warnings, warningFromError(ServiceCommons, "direct_media", err))
		return
	}
	for _, file := range files {
		if file.Missing {
			result.Warnings = append(result.Warnings, Warning{Service: ServiceCommons, Code: "file_missing", Message: "Commons file not found: " + file.Title})
			continue
		}
		media := mediaFromCommons(file, MediaKindOther)
		for _, ref := range refs {
			if fileMatchesTitle(file.Title, file.Aliases, ref.title) {
				media.Sources = appendUniqueSource(media.Sources, ref.source)
			}
		}
		if len(media.Sources) == 0 {
			continue
		}
		media.Kind = strongestKind(media.Sources)
		result.Media = mergeIntoMedia(result.Media, media)
	}
}

func (c *Client) resolveCategoryMedia(ctx context.Context, result *Result, cfg fetchConfig) {
	for _, reference := range result.Commons {
		if reference.Kind != "category" {
			continue
		}
		token, genericContinue := "", ""
		for pageNumber := 0; pageNumber < cfg.categoryPageLimit; pageNumber++ {
			page, err := c.commons.ListCategoryFiles(ctx, reference.Title,
				commons.CategoryLimit(minPositive(50, maxPositive(cfg.mediaLimit*2, 50))),
				commons.CategoryContinueWith(token, genericContinue), commons.CategoryThumbnailWidth(cfg.thumbnailWidth),
			)
			if err != nil {
				result.Warnings = append(result.Warnings, warningFromError(ServiceCommons, "category", err))
				break
			}
			for _, file := range page.Files {
				if file.Missing {
					continue
				}
				kind := kindFromCommons(file.MIMEType, file.MediaType)
				media := mediaFromCommons(file, kind)
				media.Sources = []MediaSource{{Service: ServiceCommons, Kind: "category", Value: reference.Title, Direct: false, MediaKind: kind, BaseScore: 250}}
				result.Media = mergeIntoMedia(result.Media, media)
			}
			token, genericContinue = page.ContinueToken, page.ContinueValue
			if token == "" || len(result.Media) >= maxPositive(cfg.mediaLimit*2, cfg.mediaLimit) {
				break
			}
		}
	}
}

func (c *Client) resolveWikipedia(ctx context.Context, result *Result, entity *wikidata.Entity, cfg fetchConfig) {
	for _, language := range c.languages {
		if len(result.Articles) >= cfg.articleLimit {
			break
		}
		sitelink, ok := entity.Sitelinks[language+"wiki"]
		if !ok {
			continue
		}
		article, err := c.wikipedia.GetSummary(ctx, language, sitelink.Title)
		if err != nil {
			result.Warnings = append(result.Warnings, warningFromError(ServiceWikipedia, "summary", err))
			continue
		}
		result.Articles = append(result.Articles, *article)
	}
}

func warningFromError(service Service, code string, err error) Warning {
	warning := Warning{Service: service, Code: code, Message: err.Error()}
	var apiError *mediawiki.APIError
	if errors.As(err, &apiError) && apiError.Code != "" {
		warning.Code = apiError.Code
	}
	return warning
}

func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}
func maxPositive(a, b int) int {
	if a > b {
		return a
	}
	return b
}
