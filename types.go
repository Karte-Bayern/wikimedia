package wikimedia

import (
	"encoding/json"
	"time"

	"github.com/karte-bayern/wikimedia/wikidata"
	"github.com/karte-bayern/wikimedia/wikipedia"
)

// Service identifies an upstream Wikimedia service.
type Service string

// Upstream service identifiers.
const (
	ServiceWikidata  Service = "wikidata"
	ServiceCommons   Service = "commons"
	ServiceWikipedia Service = "wikipedia"
)

// Warning records a recoverable enrichment failure.
type Warning struct {
	Service Service `json:"service"`
	Code    string  `json:"code,omitempty"`
	Message string  `json:"message"`
}

// Link is a normalized related URL.
type Link struct {
	Kind     string `json:"kind"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url"`
}

// CommonsReference is a category, gallery, or sitelink associated with an item.
type CommonsReference struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	URL      string `json:"url,omitempty"`
	Property string `json:"property,omitempty"`
}

// MediaSource explains why a Commons file belongs to an aggregate result.
type MediaSource struct {
	Service   Service   `json:"service"`
	Kind      string    `json:"kind"`
	Property  string    `json:"property,omitempty"`
	ClaimID   string    `json:"claim_id,omitempty"`
	Rank      string    `json:"rank,omitempty"`
	Value     string    `json:"value,omitempty"`
	Direct    bool      `json:"direct"`
	MediaKind MediaKind `json:"media_kind,omitempty"`
	BaseScore int       `json:"base_score,omitempty"`
}

// Media is normalized Commons metadata plus discovery and ranking information.
type Media struct {
	PageID           int64             `json:"page_id,omitempty"`
	Title            string            `json:"title"`
	FileName         string            `json:"file_name"`
	Aliases          []string          `json:"aliases,omitempty"`
	PageURL          string            `json:"page_url,omitempty"`
	OriginalURL      string            `json:"original_url,omitempty"`
	ThumbnailURL     string            `json:"thumbnail_url,omitempty"`
	Width            int               `json:"width,omitempty"`
	Height           int               `json:"height,omitempty"`
	ThumbnailWidth   int               `json:"thumbnail_width,omitempty"`
	ThumbnailHeight  int               `json:"thumbnail_height,omitempty"`
	Size             int64             `json:"size,omitempty"`
	SHA1             string            `json:"sha1,omitempty"`
	MIMEType         string            `json:"mime_type,omitempty"`
	MediaType        string            `json:"media_type,omitempty"`
	Kind             MediaKind         `json:"kind"`
	Timestamp        *time.Time        `json:"timestamp,omitempty"`
	Uploader         string            `json:"uploader,omitempty"`
	Description      string            `json:"description,omitempty"`
	ObjectName       string            `json:"object_name,omitempty"`
	Artist           string            `json:"artist,omitempty"`
	Credit           string            `json:"credit,omitempty"`
	Attribution      string            `json:"attribution,omitempty"`
	License          string            `json:"license,omitempty"`
	LicenseShortName string            `json:"license_short_name,omitempty"`
	LicenseURL       string            `json:"license_url,omitempty"`
	UsageTerms       string            `json:"usage_terms,omitempty"`
	Restrictions     string            `json:"restrictions,omitempty"`
	Copyrighted      string            `json:"copyrighted,omitempty"`
	DateTimeOriginal string            `json:"date_time_original,omitempty"`
	Sources          []MediaSource     `json:"sources"`
	Score            int               `json:"score"`
	Primary          bool              `json:"primary,omitempty"`
	ExtendedMetadata map[string]string `json:"extended_metadata,omitempty"`
	CommonMetadata   json.RawMessage   `json:"common_metadata,omitempty"`
}

// Result is the normalized aggregate for one Wikidata item.
type Result struct {
	ID           string                       `json:"id"`
	EntityURL    string                       `json:"entity_url"`
	Label        string                       `json:"label,omitempty"`
	Description  string                       `json:"description,omitempty"`
	Aliases      []string                     `json:"aliases,omitempty"`
	Labels       map[string]string            `json:"labels,omitempty"`
	Descriptions map[string]string            `json:"descriptions,omitempty"`
	Coordinates  *wikidata.CoordinateValue    `json:"coordinates,omitempty"`
	Claims       map[string][]wikidata.Claim  `json:"claims,omitempty"`
	Sitelinks    map[string]wikidata.Sitelink `json:"sitelinks,omitempty"`
	Commons      []CommonsReference           `json:"commons,omitempty"`
	Media        []Media                      `json:"media,omitempty"`
	Articles     []wikipedia.Article          `json:"articles,omitempty"`
	Links        []Link                       `json:"links,omitempty"`
	Warnings     []Warning                    `json:"warnings,omitempty"`
	RawEntity    json.RawMessage              `json:"raw_entity,omitempty"`
	FetchedAt    time.Time                    `json:"fetched_at"`
}

// PrimaryMedia returns the selected primary media item, if any.
func (r *Result) PrimaryMedia() *Media {
	if r == nil {
		return nil
	}
	for index := range r.Media {
		if r.Media[index].Primary {
			return &r.Media[index]
		}
	}
	return nil
}
