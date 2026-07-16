package wikimedia

// MediaKind classifies how a media item should be presented.
type MediaKind string

// Supported media presentation roles.
const (
	MediaKindImage      MediaKind = "image"
	MediaKindBanner     MediaKind = "banner"
	MediaKindMap        MediaKind = "map"
	MediaKindLogo       MediaKind = "logo"
	MediaKindCoatOfArms MediaKind = "coat_of_arms"
	MediaKindFlag       MediaKind = "flag"
	MediaKindSignature  MediaKind = "signature"
	MediaKindVideo      MediaKind = "video"
	MediaKindAudio      MediaKind = "audio"
	MediaKindOther      MediaKind = "other"
)

// MediaProperty maps one Wikidata property to a media role and base score.
type MediaProperty struct {
	ID        string    `json:"id"`
	Kind      MediaKind `json:"kind"`
	BaseScore int       `json:"base_score"`
}

// DefaultMediaProperties returns a copy of the built-in registry.
func DefaultMediaProperties() []MediaProperty {
	return []MediaProperty{
		{ID: "P18", Kind: MediaKindImage, BaseScore: 1000},
		{ID: "P948", Kind: MediaKindBanner, BaseScore: 700},
		{ID: "P242", Kind: MediaKindMap, BaseScore: 500},
		{ID: "P154", Kind: MediaKindLogo, BaseScore: 350},
		{ID: "P94", Kind: MediaKindCoatOfArms, BaseScore: 300},
		{ID: "P41", Kind: MediaKindFlag, BaseScore: 250},
		{ID: "P109", Kind: MediaKindSignature, BaseScore: 200},
		{ID: "P10", Kind: MediaKindVideo, BaseScore: 150},
		{ID: "P51", Kind: MediaKindAudio, BaseScore: 100},
	}
}
