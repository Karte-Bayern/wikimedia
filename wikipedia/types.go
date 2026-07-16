package wikipedia

// Thumbnail is a page image thumbnail.
type Thumbnail struct {
	Source string `json:"source,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Article is a normalized Wikipedia introductory extract.
type Article struct {
	Language    string     `json:"language"`
	PageID      int64      `json:"page_id,omitempty"`
	Title       string     `json:"title"`
	URL         string     `json:"url,omitempty"`
	Description string     `json:"description,omitempty"`
	Extract     string     `json:"extract,omitempty"`
	Thumbnail   *Thumbnail `json:"thumbnail,omitempty"`
}
