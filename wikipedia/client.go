package wikipedia

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

var languagePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)
var extractParagraphPattern = regexp.MustCompile(`[ \t]*\n[ \t]*\n+`)

// Client reads language-specific Wikipedia introductory extracts.
type Client struct{ cfg config }

// NewClient creates a Wikipedia client.
func NewClient(options ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if err := mediawiki.ValidateUserAgent(cfg.userAgent); err != nil {
		return nil, err
	}
	if !strings.Contains(cfg.endpointTemplate, "%s") {
		return nil, errors.New("wikipedia: endpoint template requires %s")
	}
	if cfg.thumbnailWidth <= 0 {
		cfg.thumbnailWidth = 1200
	}
	return &Client{cfg: cfg}, nil
}

// GetSummary fetches a plain-text introductory extract.
func (c *Client) GetSummary(ctx context.Context, language, title string) (*Article, error) {
	if c == nil {
		return nil, errors.New("wikipedia: nil client")
	}
	language = strings.ToLower(strings.TrimSpace(language))
	if !languagePattern.MatchString(language) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidLanguage, language)
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, fmt.Errorf("%w: empty title", ErrNotFound)
	}
	api, err := mediawiki.NewClient(fmt.Sprintf(c.cfg.endpointTemplate, language),
		mediawiki.WithHTTPClient(c.cfg.httpClient), mediawiki.WithUserAgent(c.cfg.userAgent),
		mediawiki.WithMaxLag(c.cfg.maxLag), mediawiki.WithMaxResponseBytes(c.cfg.maxResponseBytes),
		mediawiki.WithRetryPolicy(c.cfg.retry), mediawiki.WithCache(c.cfg.cache, c.cfg.cacheTTL),
	)
	if err != nil {
		return nil, err
	}
	parameters := url.Values{
		"action": {"query"}, "prop": {"extracts|pageimages|info|description"},
		"titles": {title}, "redirects": {"1"}, "exintro": {"1"}, "explaintext": {"1"},
		"piprop": {"thumbnail|original|name"}, "pithumbsize": {fmt.Sprintf("%d", c.cfg.thumbnailWidth)},
		"inprop": {"url"},
	}
	if c.cfg.extractSentences > 0 {
		parameters.Set("exsentences", fmt.Sprintf("%d", c.cfg.extractSentences))
	}
	var envelope struct {
		Query struct {
			Pages []struct {
				PageID      int64      `json:"pageid"`
				Title       string     `json:"title"`
				Missing     bool       `json:"missing,omitempty"`
				FullURL     string     `json:"fullurl"`
				Description string     `json:"description"`
				Extract     string     `json:"extract"`
				ImageTitle  string     `json:"pageimage"`
				Thumbnail   *Thumbnail `json:"thumbnail,omitempty"`
				Original    *Thumbnail `json:"original,omitempty"`
			} `json:"pages"`
		} `json:"query"`
	}
	if _, err := api.Query(ctx, parameters, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Query.Pages) == 0 || envelope.Query.Pages[0].Missing {
		return nil, fmt.Errorf("%w: %s:%s", ErrNotFound, language, title)
	}
	page := envelope.Query.Pages[0]
	return &Article{
		Language: language, PageID: page.PageID, Title: page.Title, URL: page.FullURL,
		Description: strings.TrimSpace(page.Description), Extract: cleanExtract(page.Extract), ImageTitle: page.ImageTitle,
		Thumbnail: page.Thumbnail, Original: page.Original,
	}, nil
}

func cleanExtract(value string) string {
	paragraphs := extractParagraphPattern.Split(strings.ReplaceAll(value, "\r\n", "\n"), -1)
	clean := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if paragraph = strings.Join(strings.Fields(paragraph), " "); paragraph != "" {
			clean = append(clean, paragraph)
		}
	}
	return strings.Join(clean, "\n\n")
}
