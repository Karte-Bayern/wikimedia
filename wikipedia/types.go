package wikipedia

// Thumbnail describes either a page image thumbnail or its original file.
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
	ImageTitle  string     `json:"image_title,omitempty"`
	Thumbnail   *Thumbnail `json:"thumbnail,omitempty"`
	Original    *Thumbnail `json:"original,omitempty"`
}
