package download

import "time"

// Source describes one remotely hosted media representation.
type Source struct {
	URL          string `json:"url"`
	FileName     string `json:"file_name,omitempty"`
	MIMEType     string `json:"mime_type,omitempty"`
	Size         int64  `json:"size,omitempty"`
	SHA1Base36   string `json:"sha1_base36,omitempty"`
	CommonsTitle string `json:"commons_title,omitempty"`
}

// File describes a committed local download.
type File struct {
	Path         string    `json:"path"`
	FileName     string    `json:"file_name"`
	SourceURL    string    `json:"source_url"`
	CommonsTitle string    `json:"commons_title,omitempty"`
	MIMEType     string    `json:"mime_type,omitempty"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256"`
	SHA1Base36   string    `json:"sha1_base36"`
	DownloadedAt time.Time `json:"downloaded_at"`
}
