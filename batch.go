package wikimedia

import (
	"context"
	"errors"
	"strings"

	"github.com/karte-bayern/wikimedia/wikidata"
)

// FetchManyItem records the outcome for one input reference. Results remain in
// the same order as their input references. Err is provided for programmatic
// handling; Error makes partial failures available in JSON output.
type FetchManyItem struct {
	Reference string  `json:"reference"`
	Result    *Result `json:"result,omitempty"`
	Error     string  `json:"error,omitempty"`
	Err       error   `json:"-"`
}

// FetchMany fetches several Wikidata IDs or supported references. Direct Q IDs
// share a single wbgetentities batch request; failures of individual references
// are retained in their corresponding FetchManyItem instead of discarding the
// rest of the batch.
func (c *Client) FetchMany(ctx context.Context, references []string, options ...FetchOption) ([]FetchManyItem, error) {
	if c == nil || c.wikidata == nil {
		return nil, errors.New("wikimedia: nil client")
	}
	items := make([]FetchManyItem, len(references))
	cfg := newFetchConfig(options)
	indices := make(map[string][]int)
	ids := make([]string, 0, len(references))
	for index, reference := range references {
		reference = strings.TrimSpace(reference)
		items[index].Reference = reference
		if wikidata.ValidItemID(reference) {
			if _, seen := indices[reference]; !seen {
				ids = append(ids, reference)
			}
			indices[reference] = append(indices[reference], index)
		}
	}
	if len(ids) > 0 {
		entities, err := c.wikidata.GetEntities(ctx, ids)
		for _, id := range ids {
			for _, index := range indices[id] {
				if err != nil {
					setFetchManyError(&items[index], err)
					continue
				}
				entity, found := entities[id]
				if !found || entity.Missing {
					setFetchManyError(&items[index], ErrNotFound)
					continue
				}
				copy := entity
				result, fetchErr := c.fetchEntity(ctx, &copy, cfg)
				if fetchErr != nil {
					setFetchManyError(&items[index], fetchErr)
					continue
				}
				items[index].Result = result
			}
		}
	}
	for index := range items {
		if items[index].Result != nil || items[index].Err != nil {
			continue
		}
		result, err := c.FetchByReference(ctx, items[index].Reference, options...)
		if err != nil {
			setFetchManyError(&items[index], err)
			continue
		}
		items[index].Result = result
	}
	if err := ctx.Err(); err != nil {
		return items, err
	}
	return items, nil
}

func setFetchManyError(item *FetchManyItem, err error) {
	item.Err = err
	if err != nil {
		item.Error = err.Error()
	}
}
