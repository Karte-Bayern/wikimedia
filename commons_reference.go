package wikimedia

import (
	"context"
	"errors"
	"strings"

	"github.com/karte-bayern/wikimedia/commons"
)

// FetchByCommonsFile fetches normalized metadata for one Commons file without
// requiring a linked Wikidata item.
func (c *Client) FetchByCommonsFile(ctx context.Context, title string, options ...FetchOption) (*Result, error) {
	if c == nil || c.commons == nil {
		return nil, errors.New("wikimedia: nil client")
	}
	cfg := newFetchConfig(options)
	file, err := c.commons.GetFile(ctx, title, commons.FileThumbnailWidth(cfg.thumbnailWidth))
	if err != nil {
		return nil, err
	}
	result := &Result{
		ID: file.Title, EntityURL: file.PageURL, Label: file.FileName,
		Commons: []CommonsReference{{Kind: "file", Title: file.Title, URL: file.PageURL}},
		Links:   []Link{{Kind: "commons", Title: file.Title, URL: file.PageURL}}, FetchedAt: c.now().UTC(),
	}
	if cfg.mediaLimit > 0 {
		kind := kindFromCommons(file.MIMEType, file.MediaType)
		media := mediaFromCommons(*file, kind)
		media.Sources = []MediaSource{{Service: ServiceCommons, Kind: "file", Value: file.Title, Direct: true, MediaKind: kind, BaseScore: 1000}}
		result.Media = rankAndLimitMedia([]Media{media}, cfg.mediaLimit)
	}
	return result, nil
}

// FetchByCommonsCategory lists direct media from one Commons category without
// requiring a linked Wikidata item. Category pagination remains bounded by
// WithCategoryPageLimit and WithMediaLimit.
func (c *Client) FetchByCommonsCategory(ctx context.Context, category string, options ...FetchOption) (*Result, error) {
	if c == nil || c.commons == nil {
		return nil, errors.New("wikimedia: nil client")
	}
	cfg := newFetchConfig(options)
	category = normalizedCommonsCategoryTitle(category)
	if category == "" {
		return nil, commons.ErrInvalidTitle
	}
	result := &Result{
		ID: category, EntityURL: commonsPageURL(category), Label: strings.TrimSpace(strings.TrimPrefix(category, "Category:")),
		Commons: []CommonsReference{{Kind: "category", Title: category, URL: commonsPageURL(category)}},
		Links:   []Link{{Kind: "commons", Title: category, URL: commonsPageURL(category)}}, FetchedAt: c.now().UTC(),
	}
	if cfg.mediaLimit == 0 {
		return result, nil
	}
	token, genericContinue := "", ""
	for pageNumber := 0; pageNumber < cfg.categoryPageLimit; pageNumber++ {
		page, err := c.commons.ListCategoryFiles(ctx, category,
			commons.CategoryLimit(minPositive(50, maxPositive(cfg.mediaLimit*2, 50))),
			commons.CategoryContinueWith(token, genericContinue), commons.CategoryThumbnailWidth(cfg.thumbnailWidth),
		)
		if err != nil {
			return nil, err
		}
		for _, file := range page.Files {
			if file.Missing {
				continue
			}
			kind := kindFromCommons(file.MIMEType, file.MediaType)
			media := mediaFromCommons(file, kind)
			media.Sources = []MediaSource{{Service: ServiceCommons, Kind: "category", Value: category, Direct: false, MediaKind: kind, BaseScore: 250}}
			result.Media = mergeIntoMedia(result.Media, media)
		}
		token, genericContinue = page.ContinueToken, page.ContinueValue
		if token == "" || len(result.Media) >= maxPositive(cfg.mediaLimit*2, cfg.mediaLimit) {
			break
		}
	}
	result.Media = rankAndLimitMedia(result.Media, cfg.mediaLimit)
	return result, nil
}

func normalizedCommonsCategoryTitle(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "category:") {
		return "Category:" + strings.TrimSpace(value[len("category:"):])
	}
	return "Category:" + value
}
