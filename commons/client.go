package commons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/karte-bayern/wikimedia/mediawiki"
)

var htmlTagPattern = regexp.MustCompile(`(?s)<[^>]*>`)

var extMetadataFields = []string{
	"ImageDescription", "ObjectName", "Artist", "Credit", "Attribution",
	"License", "LicenseShortName", "LicenseUrl", "UsageTerms", "Restrictions",
	"Copyrighted", "DateTimeOriginal", "Categories", "Assessments",
}

// Client reads Wikimedia Commons file and category metadata.
type Client struct {
	api            *mediawiki.Client
	language       string
	thumbnailWidth int
	batchSize      int
}

// NewClient creates a Commons client.
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
	if cfg.thumbnailWidth <= 0 {
		cfg.thumbnailWidth = 1200
	}
	if cfg.batchSize <= 0 || cfg.batchSize > 50 {
		cfg.batchSize = 10
	}
	if strings.TrimSpace(cfg.language) == "" {
		cfg.language = "en"
	}
	return &Client{api: api, language: cfg.language, thumbnailWidth: cfg.thumbnailWidth, batchSize: cfg.batchSize}, nil
}

// GetFile reads one Commons file.
func (c *Client) GetFile(ctx context.Context, name string, options ...FileOption) (*File, error) {
	files, err := c.GetFiles(ctx, []string{name}, options...)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 || files[0].Missing {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	file := files[0]
	return &file, nil
}

// GetFiles resolves Commons files in conservative batches.
func (c *Client) GetFiles(ctx context.Context, names []string, options ...FileOption) ([]File, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("commons: nil client")
	}
	cfg := fileConfig{language: c.language, thumbnailWidth: c.thumbnailWidth}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	clean := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, raw := range names {
		title := fileTitle(raw)
		if title == "" {
			return nil, fmt.Errorf("%w: %q", ErrInvalidTitle, raw)
		}
		key := normalizedTitleKey(title)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		clean = append(clean, title)
	}
	var result []File
	for start := 0; start < len(clean); start += c.batchSize {
		end := start + c.batchSize
		if end > len(clean) {
			end = len(clean)
		}
		files, err := c.getFilesBatch(ctx, clean[start:end], cfg)
		if err != nil {
			return nil, err
		}
		result = append(result, files...)
	}
	return result, nil
}

func (c *Client) getFilesBatch(ctx context.Context, titles []string, cfg fileConfig) ([]File, error) {
	params := fileParameters(cfg.language, cfg.thumbnailWidth, cfg.commonMetadata)
	params.Set("titles", strings.Join(titles, "|"))
	params.Set("redirects", "1")
	var envelope queryEnvelope
	if _, err := c.api.Query(ctx, params, &envelope); err != nil {
		return nil, err
	}
	files := filesFromQuery(envelope.Query)
	// Keep request order even after normalization or redirects.
	byAlias := map[string]int{}
	for index := range files {
		byAlias[normalizedTitleKey(files[index].Title)] = index
		for _, alias := range files[index].Aliases {
			byAlias[normalizedTitleKey(alias)] = index
		}
	}
	// Preserve each caller-supplied title verbatim, even when MediaWiki's
	// normalization differs only in capitalization and therefore has the same
	// normalized lookup key.
	for _, title := range titles {
		if index, ok := byAlias[normalizedTitleKey(title)]; ok {
			files[index].Aliases = appendUniqueTitle(files[index].Aliases, title)
		}
	}
	ordered := make([]File, 0, len(titles))
	used := map[int]struct{}{}
	for _, title := range titles {
		if index, ok := byAlias[normalizedTitleKey(title)]; ok {
			if _, already := used[index]; !already {
				ordered = append(ordered, files[index])
				used[index] = struct{}{}
			}
		} else {
			ordered = append(ordered, File{Title: title, FileName: strings.TrimPrefix(title, "File:"), Missing: true, Aliases: []string{title}})
		}
	}
	for index, file := range files {
		if _, ok := used[index]; !ok {
			ordered = append(ordered, file)
		}
	}
	return ordered, nil
}

// ListCategoryFiles lists one page of direct file members. It does not recurse.
func (c *Client) ListCategoryFiles(ctx context.Context, category string, options ...CategoryOption) (*FilePage, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("commons: nil client")
	}
	category = categoryTitle(category)
	if category == "" {
		return nil, ErrInvalidTitle
	}
	cfg := categoryConfig{language: c.language, thumbnailWidth: c.thumbnailWidth, limit: 50}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if cfg.limit <= 0 || cfg.limit > 500 {
		cfg.limit = 50
	}
	params := fileParameters(cfg.language, cfg.thumbnailWidth, cfg.commonMetadata)
	params.Set("generator", "categorymembers")
	params.Set("gcmtitle", category)
	params.Set("gcmtype", "file")
	params.Set("gcmlimit", fmt.Sprintf("%d", cfg.limit))
	if cfg.continueToken != "" {
		params.Set("gcmcontinue", cfg.continueToken)
		if cfg.continueValue != "" {
			params.Set("continue", cfg.continueValue)
		}
	}
	var envelope queryEnvelope
	if _, err := c.api.Query(ctx, params, &envelope); err != nil {
		return nil, err
	}
	files := filesFromQuery(envelope.Query)
	sort.Slice(files, func(i, j int) bool { return files[i].Title < files[j].Title })
	return &FilePage{
		Category: category, Files: files,
		ContinueToken: envelope.Continue.GCMContinue,
		ContinueValue: envelope.Continue.Continue,
	}, nil
}

func fileParameters(language string, width int, common bool) url.Values {
	iiprop := "url|size|sha1|mime|mediatype|user|timestamp|extmetadata"
	if common {
		iiprop += "|commonmetadata"
	}
	return url.Values{
		"action": {"query"}, "prop": {"imageinfo"}, "iiprop": {iiprop},
		"iiurlwidth":            {fmt.Sprintf("%d", width)},
		"iiextmetadatalanguage": {language},
		"iiextmetadatafilter":   {strings.Join(extMetadataFields, "|")},
	}
}

type titleMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type queryEnvelope struct {
	Continue struct {
		GCMContinue string `json:"gcmcontinue"`
		Continue    string `json:"continue"`
	} `json:"continue"`
	Query struct {
		Normalized []titleMapping `json:"normalized"`
		Redirects  []titleMapping `json:"redirects"`
		Pages      []queryPage    `json:"pages"`
	} `json:"query"`
}
type queryPage struct {
	PageID    int64       `json:"pageid"`
	Namespace int         `json:"ns"`
	Title     string      `json:"title"`
	Missing   bool        `json:"missing,omitempty"`
	ImageInfo []imageInfo `json:"imageinfo,omitempty"`
}
type imageInfo struct {
	Timestamp      string                   `json:"timestamp"`
	User           string                   `json:"user"`
	Size           int64                    `json:"size"`
	Width          int                      `json:"width"`
	Height         int                      `json:"height"`
	SHA1           string                   `json:"sha1"`
	URL            string                   `json:"url"`
	DescriptionURL string                   `json:"descriptionurl"`
	ThumbURL       string                   `json:"thumburl"`
	ThumbWidth     int                      `json:"thumbwidth"`
	ThumbHeight    int                      `json:"thumbheight"`
	MIME           string                   `json:"mime"`
	MediaType      string                   `json:"mediatype"`
	ExtMetadata    map[string]MetadataValue `json:"extmetadata"`
	CommonMetadata json.RawMessage          `json:"commonmetadata"`
}

func filesFromQuery(query struct {
	Normalized []titleMapping `json:"normalized"`
	Redirects  []titleMapping `json:"redirects"`
	Pages      []queryPage    `json:"pages"`
}) []File {
	aliases := resolvedAliases(query.Normalized, query.Redirects)
	files := make([]File, 0, len(query.Pages))
	for _, page := range query.Pages {
		file := File{PageID: page.PageID, Namespace: page.Namespace, Title: page.Title, FileName: strings.TrimPrefix(page.Title, "File:"), Missing: page.Missing}
		file.Aliases = append([]string(nil), aliases[normalizedTitleKey(page.Title)]...)
		if len(page.ImageInfo) > 0 {
			info := page.ImageInfo[0]
			file.PageURL, file.OriginalURL, file.ThumbnailURL = info.DescriptionURL, info.URL, info.ThumbURL
			file.Width, file.Height, file.ThumbnailWidth, file.ThumbnailHeight = info.Width, info.Height, info.ThumbWidth, info.ThumbHeight
			file.Size, file.SHA1, file.MIMEType, file.MediaType = info.Size, info.SHA1, info.MIME, info.MediaType
			file.Uploader = info.User
			if timestamp, err := time.Parse(time.RFC3339, info.Timestamp); err == nil {
				file.Timestamp = &timestamp
			}
			file.ExtendedMetadata = cloneMetadata(info.ExtMetadata)
			file.CommonMetadata = append(json.RawMessage(nil), info.CommonMetadata...)
			applyMetadata(&file)
		}
		if file.PageURL == "" && file.Title != "" {
			file.PageURL = commonsPageURL(file.Title)
		}
		files = append(files, file)
	}
	return files
}

func resolvedAliases(groups ...[]titleMapping) map[string][]string {
	edges := map[string]string{}
	titles := map[string]string{}
	for _, group := range groups {
		for _, mapping := range group {
			from, to := normalizedTitleKey(mapping.From), normalizedTitleKey(mapping.To)
			if from == "" || to == "" {
				continue
			}
			edges[from] = to
			titles[from] = mapping.From
			titles[to] = mapping.To
		}
	}
	result := map[string][]string{}
	for source := range edges {
		target := source
		visited := map[string]struct{}{}
		for {
			if _, cycle := visited[target]; cycle {
				break
			}
			visited[target] = struct{}{}
			next, ok := edges[target]
			if !ok {
				break
			}
			target = next
		}
		alias := titles[source]
		if alias == "" || source == target {
			continue
		}
		result[target] = appendUniqueTitle(result[target], alias)
	}
	return result
}

func applyMetadata(file *File) {
	if file == nil {
		return
	}
	get := func(key string) string { return plainMetadata(file.ExtendedMetadata[key].Value) }
	file.Description, file.ObjectName = get("ImageDescription"), get("ObjectName")
	file.Artist, file.Credit, file.Attribution = get("Artist"), get("Credit"), get("Attribution")
	file.License, file.LicenseShortName, file.LicenseURL = get("License"), get("LicenseShortName"), strings.TrimSpace(html.UnescapeString(file.ExtendedMetadata["LicenseUrl"].Value))
	file.UsageTerms, file.Restrictions, file.Copyrighted = get("UsageTerms"), get("Restrictions"), get("Copyrighted")
	file.DateTimeOriginal, file.Categories, file.Assessments = get("DateTimeOriginal"), get("Categories"), get("Assessments")
}

func plainMetadata(value string) string {
	value = strings.ReplaceAll(value, "<br>", " ")
	value = strings.ReplaceAll(value, "<br/>", " ")
	value = strings.ReplaceAll(value, "<br />", " ")
	value = html.UnescapeString(htmlTagPattern.ReplaceAllString(value, " "))
	return strings.Join(strings.FieldsFunc(value, unicode.IsSpace), " ")
}

func cloneMetadata(source map[string]MetadataValue) map[string]MetadataValue {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]MetadataValue, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func fileTitle(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if value == "" {
		return ""
	}
	if len(value) >= 5 && strings.EqualFold(value[:5], "File:") {
		return "File:" + strings.TrimSpace(value[5:])
	}
	return "File:" + value
}

func categoryTitle(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if value == "" {
		return ""
	}
	if len(value) >= 9 && strings.EqualFold(value[:9], "Category:") {
		return "Category:" + strings.TrimSpace(value[9:])
	}
	return "Category:" + value
}

func normalizedTitleKey(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(value, "_", " ")), " "))
}
func appendUniqueTitle(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
func commonsPageURL(title string) string {
	return "https://commons.wikimedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(title, " ", "_"))
}
