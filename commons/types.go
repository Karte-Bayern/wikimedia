package commons

import (
	"encoding/json"
	"time"
)

// MetadataValue is one imageinfo extmetadata field.
type MetadataValue struct {
	Value  string `json:"value,omitempty"`
	Source string `json:"source,omitempty"`
	Hidden string `json:"hidden,omitempty"`
}

// File contains normalized Commons file metadata.
type File struct {
	PageID           int64                    `json:"page_id,omitempty"`
	Namespace        int                      `json:"namespace,omitempty"`
	Title            string                   `json:"title"`
	FileName         string                   `json:"file_name"`
	Aliases          []string                 `json:"aliases,omitempty"`
	Missing          bool                     `json:"missing,omitempty"`
	PageURL          string                   `json:"page_url,omitempty"`
	OriginalURL      string                   `json:"original_url,omitempty"`
	ThumbnailURL     string                   `json:"thumbnail_url,omitempty"`
	Width            int                      `json:"width,omitempty"`
	Height           int                      `json:"height,omitempty"`
	ThumbnailWidth   int                      `json:"thumbnail_width,omitempty"`
	ThumbnailHeight  int                      `json:"thumbnail_height,omitempty"`
	Size             int64                    `json:"size,omitempty"`
	SHA1             string                   `json:"sha1,omitempty"`
	MIMEType         string                   `json:"mime_type,omitempty"`
	MediaType        string                   `json:"media_type,omitempty"`
	Timestamp        *time.Time               `json:"timestamp,omitempty"`
	Uploader         string                   `json:"uploader,omitempty"`
	Description      string                   `json:"description,omitempty"`
	ObjectName       string                   `json:"object_name,omitempty"`
	Artist           string                   `json:"artist,omitempty"`
	Credit           string                   `json:"credit,omitempty"`
	Attribution      string                   `json:"attribution,omitempty"`
	License          string                   `json:"license,omitempty"`
	LicenseShortName string                   `json:"license_short_name,omitempty"`
	LicenseURL       string                   `json:"license_url,omitempty"`
	UsageTerms       string                   `json:"usage_terms,omitempty"`
	Restrictions     string                   `json:"restrictions,omitempty"`
	Copyrighted      string                   `json:"copyrighted,omitempty"`
	DateTimeOriginal string                   `json:"date_time_original,omitempty"`
	Categories       string                   `json:"categories,omitempty"`
	Assessments      string                   `json:"assessments,omitempty"`
	ExtendedMetadata map[string]MetadataValue `json:"extended_metadata,omitempty"`
	CommonMetadata   json.RawMessage          `json:"common_metadata,omitempty"`
}

// FilePage is one explicit page of direct category files.
type FilePage struct {
	Category      string `json:"category"`
	Files         []File `json:"files"`
	ContinueToken string `json:"continue_token,omitempty"`
	ContinueValue string `json:"continue_value,omitempty"`
}
