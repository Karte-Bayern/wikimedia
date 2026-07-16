package wikimedia

import (
	"net/url"
	"sort"
	"strings"

	"github.com/karte-bayern/wikimedia/commons"
	"github.com/karte-bayern/wikimedia/wikidata"
)

func normalizeEntity(entity *wikidata.Entity, languages []string, cfg fetchConfig) *Result {
	result := &Result{
		ID: entity.ID, EntityURL: "https://www.wikidata.org/wiki/" + entity.ID,
		Labels: textMap(entity.Labels), Descriptions: textMap(entity.Descriptions),
		Sitelinks: cloneSitelinks(entity.Sitelinks),
	}
	result.Label = preferredText(entity.Labels, languages)
	result.Description = preferredText(entity.Descriptions, languages)
	result.Aliases = preferredAliases(entity.Aliases, languages)
	if cfg.claims {
		result.Claims = filterClaims(entity.Claims, cfg.deprecated)
	}
	if cfg.rawEntity {
		result.RawEntity = append([]byte(nil), entity.Raw...)
	}
	result.Coordinates = preferredCoordinate(entity.Claims["P625"], cfg.deprecated)
	result.Commons = commonsReferences(entity, cfg.deprecated)
	result.Links = normalizedLinks(entity, cfg.deprecated)
	return result
}

func textMap(source map[string]wikidata.Text) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for language, value := range source {
		result[language] = value.Value
	}
	return result
}
func preferredText(source map[string]wikidata.Text, languages []string) string {
	for _, language := range languages {
		if value, ok := source[language]; ok && value.Value != "" {
			return value.Value
		}
	}
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if source[key].Value != "" {
			return source[key].Value
		}
	}
	return ""
}
func preferredAliases(source map[string][]wikidata.Text, languages []string) []string {
	for _, language := range languages {
		if values := source[language]; len(values) > 0 {
			result := make([]string, 0, len(values))
			for _, value := range values {
				if value.Value != "" {
					result = append(result, value.Value)
				}
			}
			return result
		}
	}
	return nil
}
func cloneSitelinks(source map[string]wikidata.Sitelink) map[string]wikidata.Sitelink {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]wikidata.Sitelink, len(source))
	for key, value := range source {
		value.Badges = append([]string(nil), value.Badges...)
		value.URL = sitelinkURL(key, value.Title)
		result[key] = value
	}
	return result
}
func filterClaims(source map[string][]wikidata.Claim, includeDeprecated bool) map[string][]wikidata.Claim {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string][]wikidata.Claim, len(source))
	for property, claims := range source {
		filtered := make([]wikidata.Claim, 0, len(claims))
		for _, claim := range claims {
			if claim.Rank != "deprecated" || includeDeprecated {
				filtered = append(filtered, claim)
			}
		}
		if len(filtered) > 0 {
			result[property] = filtered
		}
	}
	return result
}
func preferredCoordinate(claims []wikidata.Claim, includeDeprecated bool) *wikidata.CoordinateValue {
	for _, rank := range []string{"preferred", "normal", "deprecated"} {
		if rank == "deprecated" && !includeDeprecated {
			continue
		}
		for _, claim := range claims {
			if claim.Rank == rank {
				if value, ok := claim.MainSnak.CoordinateValue(); ok {
					copy := value
					return &copy
				}
			}
		}
	}
	return nil
}

func commonsReferences(entity *wikidata.Entity, includeDeprecated bool) []CommonsReference {
	var result []CommonsReference
	for _, claim := range entity.Claims["P373"] {
		if claim.Rank == "deprecated" && !includeDeprecated {
			continue
		}
		if value, ok := claim.MainSnak.StringValue(); ok && strings.TrimSpace(value) != "" {
			result = appendCommonsReference(result, CommonsReference{Kind: "category", Title: categoryTitle(value), URL: commonsPageURL(categoryTitle(value)), Property: "P373"})
		}
	}
	for _, claim := range entity.Claims["P935"] {
		if claim.Rank == "deprecated" && !includeDeprecated {
			continue
		}
		if value, ok := claim.MainSnak.StringValue(); ok && strings.TrimSpace(value) != "" {
			result = appendCommonsReference(result, CommonsReference{Kind: "gallery", Title: strings.TrimSpace(value), URL: commonsPageURL(value), Property: "P935"})
		}
	}
	if sitelink, ok := entity.Sitelinks["commonswiki"]; ok {
		kind := "page"
		if strings.HasPrefix(strings.ToLower(sitelink.Title), "category:") {
			kind = "category"
		}
		result = appendCommonsReference(result, CommonsReference{Kind: kind, Title: sitelink.Title, URL: commonsPageURL(sitelink.Title)})
	}
	return result
}
func appendCommonsReference(values []CommonsReference, value CommonsReference) []CommonsReference {
	key := value.Kind + "\x00" + mediaTitleKey(value.Title)
	for _, existing := range values {
		if existing.Kind+"\x00"+mediaTitleKey(existing.Title) == key {
			return values
		}
	}
	return append(values, value)
}
func normalizedLinks(entity *wikidata.Entity, includeDeprecated bool) []Link {
	result := []Link{{Kind: "wikidata", Title: entity.ID, URL: "https://www.wikidata.org/wiki/" + entity.ID}}
	keys := make([]string, 0, len(entity.Sitelinks))
	for key := range entity.Sitelinks {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, site := range keys {
		sitelink := entity.Sitelinks[site]
		linkURL := sitelinkURL(site, sitelink.Title)
		if linkURL == "" {
			continue
		}
		kind, language := "wikimedia", ""
		if site == "commonswiki" {
			kind = "commons"
		} else if strings.HasSuffix(site, "wiki") {
			kind = "wikipedia"
			language = strings.TrimSuffix(site, "wiki")
		}
		result = append(result, Link{Kind: kind, Language: language, Title: sitelink.Title, URL: linkURL})
	}
	for _, claim := range entity.Claims["P856"] {
		if claim.Rank == "deprecated" && !includeDeprecated {
			continue
		}
		if value, ok := claim.MainSnak.StringValue(); ok {
			if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
				result = append(result, Link{Kind: "official_website", URL: value})
			}
		}
	}
	return deduplicateLinks(result)
}
func deduplicateLinks(values []Link) []Link {
	seen := map[string]struct{}{}
	result := values[:0]
	for _, value := range values {
		if _, ok := seen[value.URL]; ok {
			continue
		}
		seen[value.URL] = struct{}{}
		result = append(result, value)
	}
	return result
}
func sitelinkURL(site, title string) string {
	escaped := url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	special := map[string]string{
		"commonswiki":    "https://commons.wikimedia.org/wiki/",
		"wikidatawiki":   "https://www.wikidata.org/wiki/",
		"mediawikiwiki":  "https://www.mediawiki.org/wiki/",
		"specieswiki":    "https://species.wikimedia.org/wiki/",
		"metawiki":       "https://meta.wikimedia.org/wiki/",
		"incubatorwiki":  "https://incubator.wikimedia.org/wiki/",
		"outreachwiki":   "https://outreach.wikimedia.org/wiki/",
		"foundationwiki": "https://foundation.wikimedia.org/wiki/",
	}
	if base, ok := special[site]; ok {
		return base + escaped
	}
	if !strings.HasSuffix(site, "wiki") {
		return ""
	}
	language := strings.ReplaceAll(strings.TrimSuffix(site, "wiki"), "_", "-")
	if language == "" || !safeHostnameLabel(language) {
		return ""
	}
	return "https://" + language + ".wikipedia.org/wiki/" + escaped
}

func safeHostnameLabel(value string) bool {
	if value == "" || value[0] == '-' || value[len(value)-1] == '-' {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
func commonsPageURL(title string) string {
	return "https://commons.wikimedia.org/wiki/" + url.PathEscape(strings.ReplaceAll(strings.TrimSpace(title), " ", "_"))
}
func categoryTitle(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	if strings.HasPrefix(strings.ToLower(value), "category:") {
		return "Category:" + strings.TrimSpace(value[len("category:"):])
	}
	return "Category:" + value
}

func mediaFromCommons(file commons.File, kind MediaKind) Media {
	extended := make(map[string]string, len(file.ExtendedMetadata))
	for key, value := range file.ExtendedMetadata {
		extended[key] = value.Value
	}
	if len(extended) == 0 {
		extended = nil
	}
	return Media{
		PageID: file.PageID, Title: file.Title, FileName: file.FileName, Aliases: append([]string(nil), file.Aliases...),
		PageURL: file.PageURL, OriginalURL: file.OriginalURL, ThumbnailURL: file.ThumbnailURL,
		Width: file.Width, Height: file.Height, ThumbnailWidth: file.ThumbnailWidth, ThumbnailHeight: file.ThumbnailHeight,
		Size: file.Size, SHA1: file.SHA1, MIMEType: file.MIMEType, MediaType: file.MediaType, Kind: kind,
		Timestamp: file.Timestamp, Uploader: file.Uploader, Description: file.Description, ObjectName: file.ObjectName,
		Artist: file.Artist, Credit: file.Credit, Attribution: file.Attribution, License: file.License,
		LicenseShortName: file.LicenseShortName, LicenseURL: file.LicenseURL, UsageTerms: file.UsageTerms,
		Restrictions: file.Restrictions, Copyrighted: file.Copyrighted, DateTimeOriginal: file.DateTimeOriginal,
		ExtendedMetadata: extended, CommonMetadata: append([]byte(nil), file.CommonMetadata...),
	}
}

func mergeIntoMedia(values []Media, candidate Media) []Media {
	for index := range values {
		if sameMedia(values[index], candidate) {
			mergeMedia(&values[index], candidate)
			return values
		}
	}
	return append(values, candidate)
}
func sameMedia(a, b Media) bool {
	if a.PageID != 0 && b.PageID != 0 {
		return a.PageID == b.PageID
	}
	if a.SHA1 != "" && b.SHA1 != "" {
		return strings.EqualFold(a.SHA1, b.SHA1)
	}
	return mediaTitleKey(a.Title) != "" && mediaTitleKey(a.Title) == mediaTitleKey(b.Title)
}
func mergeMedia(target *Media, source Media) {
	for _, discovery := range source.Sources {
		target.Sources = appendUniqueSource(target.Sources, discovery)
	}
	for _, alias := range source.Aliases {
		target.Aliases = appendUniqueString(target.Aliases, alias)
	}
	if strongestKind(target.Sources) != MediaKindOther {
		target.Kind = strongestKind(target.Sources)
	}
	if target.PageURL == "" {
		target.PageURL = source.PageURL
	}
	if target.OriginalURL == "" {
		target.OriginalURL = source.OriginalURL
	}
	if target.ThumbnailURL == "" {
		target.ThumbnailURL = source.ThumbnailURL
	}
	if target.Description == "" {
		target.Description = source.Description
	}
	if target.Artist == "" {
		target.Artist = source.Artist
	}
	if target.Credit == "" {
		target.Credit = source.Credit
	}
	if target.Attribution == "" {
		target.Attribution = source.Attribution
	}
	if target.LicenseShortName == "" {
		target.LicenseShortName = source.LicenseShortName
	}
	if target.LicenseURL == "" {
		target.LicenseURL = source.LicenseURL
	}
}
func appendUniqueSource(values []MediaSource, value MediaSource) []MediaSource {
	for _, existing := range values {
		if existing.Service == value.Service && existing.Kind == value.Kind && existing.Property == value.Property && existing.ClaimID == value.ClaimID && mediaTitleKey(existing.Value) == mediaTitleKey(value.Value) {
			return values
		}
	}
	return append(values, value)
}
func appendUniqueString(values []string, value string) []string {
	key := mediaTitleKey(value)
	for _, existing := range values {
		if mediaTitleKey(existing) == key {
			return values
		}
	}
	return append(values, value)
}
func strongestKind(sources []MediaSource) MediaKind {
	bestKind, bestScore := MediaKindOther, -1<<30
	for _, source := range sources {
		if source.BaseScore > bestScore {
			bestScore, bestKind = source.BaseScore, source.MediaKind
		}
	}
	return bestKind
}
func kindFromCommons(mime, mediaType string) MediaKind {
	mime = strings.ToLower(mime)
	mediaType = strings.ToLower(mediaType)
	if strings.HasPrefix(mime, "audio/") || mediaType == "audio" {
		return MediaKindAudio
	}
	if strings.HasPrefix(mime, "video/") || mediaType == "video" {
		return MediaKindVideo
	}
	if strings.HasPrefix(mime, "image/") || mediaType == "bitmap" || mediaType == "drawing" {
		return MediaKindImage
	}
	return MediaKindOther
}
func mediaTitleKey(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
	value = strings.TrimPrefix(strings.ToLower(value), "file:")
	return strings.Join(strings.Fields(value), " ")
}
func fileMatchesTitle(title string, aliases []string, requested string) bool {
	key := mediaTitleKey(requested)
	if mediaTitleKey(title) == key {
		return true
	}
	for _, alias := range aliases {
		if mediaTitleKey(alias) == key {
			return true
		}
	}
	return false
}

func rankAndLimitMedia(values []Media, limit int) []Media {
	for index := range values {
		values[index].Score = scoreMedia(values[index])
		values[index].Primary = false
	}
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Score != values[j].Score {
			return values[i].Score > values[j].Score
		}
		if values[i].PageID != values[j].PageID {
			return values[i].PageID < values[j].PageID
		}
		return values[i].Title < values[j].Title
	})
	if limit >= 0 && len(values) > limit {
		values = values[:limit]
	}
	for index := range values {
		if isVisual(values[index]) {
			values[index].Primary = true
			break
		}
	}
	return values
}
func scoreMedia(media Media) int {
	return mediaScoreComponents(media)["total"]
}

func mediaScoreComponents(media Media) map[string]int {
	parts := map[string]int{
		"base":        0,
		"preferred":   0,
		"resolution":  0,
		"width":       0,
		"license":     0,
		"attribution": 0,
		"description": 0,
		"aspect":      0,
		"kind":        0,
	}
	baseSet := false
	for _, source := range media.Sources {
		if !baseSet || source.BaseScore > parts["base"] {
			parts["base"] = source.BaseScore
			baseSet = true
		}
		if source.Rank == "preferred" {
			parts["preferred"] = 100
		}
	}
	pixels := int64(media.Width) * int64(media.Height)
	switch {
	case pixels >= 4_000_000:
		parts["resolution"] = 140
	case pixels >= 1_000_000:
		parts["resolution"] = 90
	case pixels > 0 && pixels < 120_000:
		parts["resolution"] = -300
	}
	if media.Width >= 1200 {
		parts["width"] = 40
	}
	if media.LicenseURL != "" || media.LicenseShortName != "" {
		parts["license"] = 100
	}
	if media.Artist != "" || media.Attribution != "" {
		parts["attribution"] = 50
	}
	if media.Description != "" {
		parts["description"] = 30
	}
	if media.Width > 0 && media.Height > 0 {
		ratio := float64(media.Width) / float64(media.Height)
		if ratio >= 1.1 && ratio <= 2.2 {
			parts["aspect"] += 25
		}
		if ratio > 4 || ratio < .25 {
			parts["aspect"] -= 180
		}
	}
	switch media.Kind {
	case MediaKindLogo, MediaKindCoatOfArms, MediaKindFlag, MediaKindSignature:
		parts["kind"] = -250
	case MediaKindAudio, MediaKindVideo:
		parts["kind"] = -500
	}
	total := 0
	for name, value := range parts {
		if name != "total" {
			total += value
		}
	}
	parts["total"] = total
	return parts
}

func isVisual(media Media) bool {
	return media.Kind != MediaKindAudio && media.Kind != MediaKindVideo && (strings.HasPrefix(strings.ToLower(media.MIMEType), "image/") || media.Kind == MediaKindImage || media.Kind == MediaKindBanner || media.Kind == MediaKindMap || media.Kind == MediaKindLogo || media.Kind == MediaKindFlag || media.Kind == MediaKindCoatOfArms || media.Kind == MediaKindSignature)
}

// ExplainMediaScore returns each additive ranking contribution and the total.
func ExplainMediaScore(media Media) map[string]int {
	return mediaScoreComponents(media)
}
