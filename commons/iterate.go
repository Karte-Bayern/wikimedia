package commons

import (
	"context"
	"errors"
)

// FileVisitor receives one direct category file. Returning an error stops the
// iteration and returns that error to the caller.
type FileVisitor func(File) error

// WalkCategoryFiles follows explicit Commons category pagination for at most
// maxPages pages. It never recurses into subcategories.
func (c *Client) WalkCategoryFiles(ctx context.Context, category string, maxPages int, visit FileVisitor, options ...CategoryOption) error {
	if maxPages <= 0 {
		return errors.New("commons: category page limit must be positive")
	}
	if visit == nil {
		return errors.New("commons: nil file visitor")
	}
	token, genericContinue := "", ""
	for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
		pageOptions := append([]CategoryOption(nil), options...)
		if pageNumber > 0 || token != "" {
			pageOptions = append(pageOptions, CategoryContinueWith(token, genericContinue))
		}
		page, err := c.ListCategoryFiles(ctx, category, pageOptions...)
		if err != nil {
			return err
		}
		for _, file := range page.Files {
			if err := visit(file); err != nil {
				return err
			}
		}
		token, genericContinue = page.ContinueToken, page.ContinueValue
		if token == "" {
			return nil
		}
	}
	return nil
}

// CollectCategoryFiles returns direct category files from at most maxPages
// pages. It is a convenience wrapper around WalkCategoryFiles.
func (c *Client) CollectCategoryFiles(ctx context.Context, category string, maxPages int, options ...CategoryOption) ([]File, error) {
	files := make([]File, 0)
	err := c.WalkCategoryFiles(ctx, category, maxPages, func(file File) error {
		files = append(files, file)
		return nil
	}, options...)
	if err != nil {
		return nil, err
	}
	return files, nil
}
