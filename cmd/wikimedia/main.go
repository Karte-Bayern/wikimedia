package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/karte-bayern/wikimedia"
	"github.com/karte-bayern/wikimedia/cache"
	"github.com/karte-bayern/wikimedia/commons"
	"github.com/karte-bayern/wikimedia/download"
	"github.com/karte-bayern/wikimedia/wikidata"
)

const repositoryURL = "https://github.com/Karte-Bayern/wikimedia"

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) }

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "get":
		return runGet(ctx, args[1:], stdout, stderr)
	case "media":
		return runMedia(ctx, args[1:], stdout, stderr)
	case "download":
		return runDownload(ctx, args[1:], stdout, stderr)
	case "search":
		return runSearch(ctx, args[1:], stdout, stderr)
	case "sparql":
		return runSPARQL(ctx, args[1:], stdout, stderr)
	case "nearby":
		return runNearby(ctx, args[1:], stdout, stderr)
	case "category":
		return runCategory(ctx, args[1:], stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintf(stdout, "wikimedia %s\n", wikimedia.Version)
		return 0
	case "help", "--help", "-h":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", args[0])
		printUsage(stderr)
		return 2
	}
}

type commonFlags struct {
	languages, userAgent, cacheDir, format string
	timeout                                time.Duration
	compact                                bool
}
type fetchFlags struct {
	directMedia, categories, wikipedia, claims, raw, deprecated bool
	mediaLimit, thumbnailWidth, articleLimit, categoryPageLimit int
}

func defaultApplicationUserAgent() string {
	if value := strings.TrimSpace(os.Getenv("WIKIMEDIA_USER_AGENT")); value != "" {
		return value
	}
	return "karte-bayern-wikimedia-cli/" + wikimedia.Version + " (" + repositoryURL + ")"
}
func addCommonFlags(set *flag.FlagSet, values *commonFlags, compact bool, defaultTimeout time.Duration) {
	set.StringVar(&values.languages, "languages", "de,en", "preferred language codes, comma-separated")
	set.StringVar(&values.languages, "l", "de,en", "shorthand for --languages")
	set.StringVar(&values.userAgent, "user-agent", defaultApplicationUserAgent(), "descriptive User-Agent with contact information")
	set.StringVar(&values.userAgent, "u", defaultApplicationUserAgent(), "shorthand for --user-agent")
	set.StringVar(&values.cacheDir, "cache-dir", "", "optional persistent API cache directory")
	set.StringVar(&values.cacheDir, "C", "", "shorthand for --cache-dir")
	set.DurationVar(&values.timeout, "timeout", defaultTimeout, "overall command timeout")
	set.DurationVar(&values.timeout, "t", defaultTimeout, "shorthand for --timeout")
	if compact {
		set.BoolVar(&values.compact, "compact", false, "emit compact JSON")
		set.StringVar(&values.format, "format", "json", "output format: json, jsonl, or text")
		set.StringVar(&values.format, "F", "json", "shorthand for --format")
	}
}

func addOutputFlags(set *flag.FlagSet, output *string) {
	set.StringVar(output, "output", "", "write output to file instead of stdout")
	set.StringVar(output, "o", "", "shorthand for --output")
}
func addFetchFlags(set *flag.FlagSet, values *fetchFlags, entityOutput bool) {
	values.directMedia = true
	values.mediaLimit = 20
	values.thumbnailWidth = 1200
	values.categoryPageLimit = 3
	set.BoolVar(&values.directMedia, "media", true, "resolve direct Wikidata media")
	set.BoolVar(&values.categories, "categories", false, "include direct files from linked Commons categories")
	set.BoolVar(&values.deprecated, "deprecated", false, "include deprecated Wikidata statements")
	set.IntVar(&values.mediaLimit, "media-limit", 20, "maximum media items (0 disables media)")
	set.IntVar(&values.thumbnailWidth, "thumbnail-width", 1200, "requested Commons thumbnail width")
	set.IntVar(&values.categoryPageLimit, "category-pages", 3, "maximum pages per linked Commons category")
	if entityOutput {
		values.claims = true
		values.articleLimit = 1
		set.BoolVar(&values.wikipedia, "wikipedia", false, "load Wikipedia introductory extracts")
		set.BoolVar(&values.claims, "claims", true, "include Wikidata claims")
		set.BoolVar(&values.raw, "raw", false, "include raw Wikidata entity JSON")
		set.IntVar(&values.articleLimit, "article-limit", 1, "maximum Wikipedia extracts")
	}
}
func (f commonFlags) newClient() (*wikimedia.Client, error) {
	options := []wikimedia.Option{wikimedia.WithUserAgent(f.userAgent), wikimedia.WithLanguages(splitComma(f.languages)...)}
	if strings.TrimSpace(f.cacheDir) != "" {
		storage, err := cache.NewFilesystem(f.cacheDir)
		if err != nil {
			return nil, err
		}
		options = append(options, wikimedia.WithCache(storage, wikimedia.CacheTTLs{}))
	}
	return wikimedia.New(options...)
}
func (f fetchFlags) options() []wikimedia.FetchOption {
	return []wikimedia.FetchOption{
		wikimedia.WithDirectMedia(f.directMedia), wikimedia.WithCommonsCategories(f.categories), wikimedia.WithWikipediaSummaries(f.wikipedia),
		wikimedia.WithClaims(f.claims), wikimedia.WithRawEntity(f.raw), wikimedia.WithDeprecatedStatements(f.deprecated),
		wikimedia.WithMediaLimit(f.mediaLimit), wikimedia.WithThumbnailWidth(f.thumbnailWidth), wikimedia.WithArticleLimit(f.articleLimit), wikimedia.WithCategoryPageLimit(f.categoryPageLimit),
	}
}

func runGet(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia get", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var fetch fetchFlags
	var output, osmReference, wikipediaReference string
	addCommonFlags(set, &common, true, 45*time.Second)
	addFetchFlags(set, &fetch, true)
	addOutputFlags(set, &output)
	set.StringVar(&osmReference, "osm", "", "resolve OSM TYPE/ID (relation, way, or node)")
	set.StringVar(&wikipediaReference, "wiki", "", "resolve Wikipedia LANGUAGE:TITLE")
	set.Usage = func() {
		fmt.Fprintln(set.Output(), "usage: wikimedia get [flags] REFERENCE | --osm TYPE/ID | --wiki LANGUAGE:TITLE")
		set.PrintDefaults()
	}
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if (osmReference != "" || wikipediaReference != "") && (osmReference != "" && wikipediaReference != "" || set.NArg() != 0) {
		fmt.Fprintln(stderr, "use exactly one of REFERENCE, --osm, or --wiki")
		return 2
	}
	references := []string(nil)
	if osmReference == "" && wikipediaReference == "" {
		var ok bool
		references, ok = requireReferences(set, stderr)
		if !ok {
			return 2
		}
	} else if osmReference != "" {
		references = []string{"osm:" + osmReference}
	} else {
		references = []string{"wikipedia:" + wikipediaReference}
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	var value any
	if len(references) == 1 {
		result, err := fetchByReference(ctx, client, references[0], osmReference, wikipediaReference, fetch.options())
		if err != nil {
			return printError(stderr, err)
		}
		value = result
	} else {
		items, err := client.FetchMany(ctx, references, fetch.options()...)
		if err != nil {
			return printError(stderr, err)
		}
		value = items
	}
	if err := writeOutput(output, value, common.format, common.compact, stdout); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func fetchByReference(ctx context.Context, client *wikimedia.Client, reference, osmReference, wikipediaReference string, options []wikimedia.FetchOption) (*wikimedia.Result, error) {
	if osmReference != "" {
		kind, value, ok := strings.Cut(strings.TrimSpace(osmReference), "/")
		if !ok || value == "" {
			return nil, fmt.Errorf("OSM reference must be TYPE/ID")
		}
		return client.FetchByOSM(ctx, wikimedia.OSMType(strings.ToLower(kind)), value, options...)
	}
	if wikipediaReference != "" {
		language, title, ok := strings.Cut(strings.TrimSpace(wikipediaReference), ":")
		if !ok || title == "" {
			return nil, fmt.Errorf("Wikipedia reference must be LANGUAGE:TITLE")
		}
		return client.FetchByWikipedia(ctx, language, title, options...)
	}
	return client.FetchByReference(ctx, reference, options...)
}

func runMedia(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia media", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var fetch fetchFlags
	var output string
	addCommonFlags(set, &common, true, 45*time.Second)
	addFetchFlags(set, &fetch, false)
	addOutputFlags(set, &output)
	set.Usage = func() { fmt.Fprintln(set.Output(), "usage: wikimedia media [flags] REFERENCE"); set.PrintDefaults() }
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	reference, ok := requireOneReference(set, stderr)
	if !ok {
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	result, err := client.FetchByReference(ctx, reference, fetch.options()...)
	if err != nil {
		return printError(stderr, err)
	}
	payload := mediaOutput{ID: result.ID, Label: result.Label, Media: result.Media, Warnings: result.Warnings}
	if err := writeOutput(output, payload, common.format, common.compact, stdout); err != nil {
		return printError(stderr, err)
	}
	return 0
}

type manifest struct {
	EntityID    string              `json:"entity_id"`
	Label       string              `json:"label,omitempty"`
	Variant     string              `json:"variant"`
	FetchedAt   time.Time           `json:"fetched_at"`
	Files       []manifestFile      `json:"files"`
	Warnings    []string            `json:"warnings,omitempty"`
	APIWarnings []wikimedia.Warning `json:"api_warnings,omitempty"`
}
type manifestFile struct {
	Local            download.File `json:"local"`
	CommonsTitle     string        `json:"commons_title"`
	CommonsPageURL   string        `json:"commons_page_url,omitempty"`
	OriginalURL      string        `json:"original_url,omitempty"`
	ThumbnailURL     string        `json:"thumbnail_url,omitempty"`
	Artist           string        `json:"artist,omitempty"`
	Credit           string        `json:"credit,omitempty"`
	Attribution      string        `json:"attribution,omitempty"`
	License          string        `json:"license,omitempty"`
	LicenseShortName string        `json:"license_short_name,omitempty"`
	LicenseURL       string        `json:"license_url,omitempty"`
}

type mediaOutput struct {
	ID       string              `json:"id"`
	Label    string              `json:"label,omitempty"`
	Media    []wikimedia.Media   `json:"media"`
	Warnings []wikimedia.Warning `json:"warnings,omitempty"`
}

func runDownload(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia download", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var fetch fetchFlags
	var outputDir, manifestPath, variant, kind string
	var overwrite, primaryOnly, resume bool
	var concurrency int
	var maximumBytes int64
	addCommonFlags(set, &common, false, 5*time.Minute)
	addFetchFlags(set, &fetch, false)
	set.StringVar(&outputDir, "output", ".", "base output directory")
	set.StringVar(&manifestPath, "manifest", "", "manifest path (default: OUTPUT/RESOLVED-QID/manifest.json)")
	set.StringVar(&variant, "variant", "thumbnail", "download variant: thumbnail or original")
	set.StringVar(&kind, "kind", "image", "media filter: image or all")
	set.BoolVar(&primaryOnly, "primary", false, "download only selected primary media")
	set.BoolVar(&overwrite, "overwrite", false, "replace existing regular files")
	set.BoolVar(&resume, "resume", false, "reuse verified files from the previous batch manifest")
	set.IntVar(&concurrency, "concurrency", 4, "parallel downloads (1-16)")
	set.Int64Var(&maximumBytes, "max-bytes", 50<<20, "maximum bytes per file")
	set.Usage = func() { fmt.Fprintln(set.Output(), "usage: wikimedia download [flags] REFERENCE"); set.PrintDefaults() }
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	reference, ok := requireOneReference(set, stderr)
	if !ok {
		return 2
	}
	variant = strings.ToLower(strings.TrimSpace(variant))
	if variant != "thumbnail" && variant != "original" {
		fmt.Fprintln(stderr, "variant must be thumbnail or original")
		return 2
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "image" && kind != "all" {
		fmt.Fprintln(stderr, "kind must be image or all")
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	result, err := client.FetchByReference(ctx, reference, fetch.options()...)
	if err != nil {
		return printError(stderr, err)
	}
	items := filterDownloadMedia(result.Media, primaryOnly, kind)
	if len(items) == 0 {
		fmt.Fprintln(stderr, "no matching media found")
		return 1
	}
	destination := filepath.Join(outputDir, result.ID)
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return printError(stderr, err)
	}
	downloader, err := download.New(download.WithUserAgent(common.userAgent+" karte-bayern-wikimedia/"+wikimedia.Version), download.WithMaximumBytes(maximumBytes), download.WithOverwrite(overwrite))
	if err != nil {
		return printError(stderr, err)
	}
	manifestValue := manifest{EntityID: result.ID, Label: result.Label, Variant: variant, FetchedAt: time.Now().UTC(), APIWarnings: result.Warnings}
	failed := false
	sources := make([]download.Source, 0, len(items))
	mediaByTitle := make(map[string]wikimedia.Media, len(items))
	for _, media := range items {
		source, sourceErr := downloadSource(media, variant)
		if sourceErr != nil {
			manifestValue.Warnings = append(manifestValue.Warnings, sourceErr.Error())
			failed = true
			continue
		}
		sources = append(sources, source)
		mediaByTitle[media.Title] = media
	}
	batchManifestPath := filepath.Join(destination, ".wikimedia-downloads.json")
	batch, batchErr := downloader.DownloadBatch(ctx, sources, destination, download.WithConcurrency(concurrency), download.WithManifest(batchManifestPath), download.WithResume(resume))
	if batch == nil {
		return printError(stderr, batchErr)
	}
	if batchErr != nil {
		failed = true
	}
	for _, item := range batch.Items {
		media := mediaByTitle[item.Source.CommonsTitle]
		if item.Status == download.BatchFailed {
			manifestValue.Warnings = append(manifestValue.Warnings, media.Title+": "+item.Error)
			continue
		}
		if item.File == nil {
			continue
		}
		manifestValue.Files = append(manifestValue.Files, manifestFile{Local: *item.File, CommonsTitle: media.Title, CommonsPageURL: media.PageURL, OriginalURL: media.OriginalURL, ThumbnailURL: media.ThumbnailURL, Artist: media.Artist, Credit: media.Credit, Attribution: media.Attribution, License: media.License, LicenseShortName: media.LicenseShortName, LicenseURL: media.LicenseURL})
	}
	if manifestPath == "" {
		manifestPath = filepath.Join(destination, "manifest.json")
	}
	if err := writeJSONFile(manifestPath, manifestValue, false); err != nil {
		return printError(stderr, err)
	}
	fmt.Fprintf(stdout, "downloaded %d file(s); manifest: %s\n", len(manifestValue.Files), manifestPath)
	if failed {
		return 1
	}
	return 0
}

func downloadSource(media wikimedia.Media, variant string) (download.Source, error) {
	if variant == "original" {
		if media.OriginalURL == "" {
			return download.Source{}, fmt.Errorf("%s: no original URL", media.Title)
		}
		return download.Source{URL: media.OriginalURL, FileName: media.FileName, MIMEType: media.MIMEType, Size: media.Size, SHA1Base36: media.SHA1, CommonsTitle: media.Title}, nil
	}
	if media.ThumbnailURL == "" {
		return download.Source{}, fmt.Errorf("%s: no thumbnail URL", media.Title)
	}
	parsed, err := url.Parse(media.ThumbnailURL)
	if err != nil {
		return download.Source{}, err
	}
	return download.Source{URL: media.ThumbnailURL, FileName: download.FileNameFromURL(parsed), CommonsTitle: media.Title}, nil
}
func filterDownloadMedia(values []wikimedia.Media, primary bool, kind string) []wikimedia.Media {
	result := make([]wikimedia.Media, 0, len(values))
	for _, value := range values {
		if primary && !value.Primary {
			continue
		}
		if kind == "image" && !isVisualMedia(value) {
			continue
		}
		result = append(result, value)
		if primary {
			break
		}
	}
	return result
}
func isVisualMedia(value wikimedia.Media) bool {
	if strings.HasPrefix(strings.ToLower(value.MIMEType), "image/") {
		return true
	}
	switch value.Kind {
	case wikimedia.MediaKindImage, wikimedia.MediaKindBanner, wikimedia.MediaKindMap, wikimedia.MediaKindLogo, wikimedia.MediaKindCoatOfArms, wikimedia.MediaKindFlag, wikimedia.MediaKindSignature:
		return true
	default:
		return false
	}
}

func runSearch(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia search", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var output, language string
	var limit int
	addCommonFlags(set, &common, true, 45*time.Second)
	set.StringVar(&language, "language", "", "result language (default: first --languages value)")
	set.IntVar(&limit, "limit", 10, "maximum item results (1-50)")
	set.IntVar(&limit, "n", 10, "shorthand for --limit")
	addOutputFlags(set, &output)
	set.Usage = func() { fmt.Fprintln(set.Output(), "usage: wikimedia search [flags] QUERY"); set.PrintDefaults() }
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if set.NArg() == 0 {
		set.Usage()
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	options := []wikidata.SearchOption{wikidata.SearchLimit(limit)}
	if strings.TrimSpace(language) != "" {
		options = append(options, wikidata.SearchLanguage(language))
	}
	results, err := client.SearchItems(ctx, strings.Join(set.Args(), " "), options...)
	if err != nil {
		return printError(stderr, err)
	}
	if err := writeOutput(output, results, common.format, common.compact, stdout); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func runSPARQL(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia sparql", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var output, query, queryFile string
	addCommonFlags(set, &common, true, 45*time.Second)
	set.StringVar(&query, "query", "", "SPARQL query text")
	set.StringVar(&query, "q", "", "shorthand for --query")
	set.StringVar(&queryFile, "file", "", "read SPARQL query text from file")
	set.StringVar(&queryFile, "f", "", "shorthand for --file")
	addOutputFlags(set, &output)
	set.Usage = func() {
		fmt.Fprintln(set.Output(), "usage: wikimedia sparql [flags] --query QUERY | --file QUERY.rq")
		set.PrintDefaults()
	}
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if query != "" && queryFile != "" || set.NArg() != 0 {
		fmt.Fprintln(stderr, "use exactly one of --query or --file")
		return 2
	}
	if queryFile != "" {
		contents, err := readSmallFile(queryFile, 1<<20)
		if err != nil {
			return printError(stderr, err)
		}
		query = contents
	}
	if strings.TrimSpace(query) == "" {
		set.Usage()
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	result, err := client.SPARQL().Query(ctx, query)
	if err != nil {
		return printError(stderr, err)
	}
	if err := writeOutput(output, result, common.format, common.compact, stdout); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func runNearby(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia nearby", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var output, bbox string
	var radius float64
	var limit int
	addCommonFlags(set, &common, true, 45*time.Second)
	set.Float64Var(&radius, "radius", 1, "search radius in kilometres")
	set.IntVar(&limit, "limit", 50, "maximum items (1-500)")
	set.IntVar(&limit, "n", 50, "shorthand for --limit")
	set.StringVar(&bbox, "bbox", "", "bounding box: south,west,north,east (instead of coordinates)")
	addOutputFlags(set, &output)
	set.Usage = func() {
		fmt.Fprintln(set.Output(), "usage: wikimedia nearby [flags] LATITUDE LONGITUDE | --bbox SOUTH,WEST,NORTH,EAST")
		set.PrintDefaults()
	}
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	request, err := parseNearbyRequest(set, bbox, radius, limit)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		set.Usage()
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	var values []wikimedia.GeoItem
	if request.boundingBox {
		values, err = client.FindInBoundingBox(ctx, request.coordinates[0], request.coordinates[1], request.coordinates[2], request.coordinates[3], request.limit)
	} else {
		values, err = client.FindNearby(ctx, request.coordinates[0], request.coordinates[1], request.radius, request.limit)
	}
	if err != nil {
		return printError(stderr, err)
	}
	if err := writeOutput(output, values, common.format, common.compact, stdout); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func runCategory(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia category", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var output string
	var pages, limit, thumbnailWidth int
	addCommonFlags(set, &common, true, 45*time.Second)
	set.IntVar(&pages, "pages", 1, "maximum Commons category pages")
	set.IntVar(&limit, "limit", 50, "maximum files per category page (1-500)")
	set.IntVar(&limit, "n", 50, "shorthand for --limit")
	set.IntVar(&thumbnailWidth, "thumbnail-width", 1200, "requested thumbnail width")
	addOutputFlags(set, &output)
	set.Usage = func() { fmt.Fprintln(set.Output(), "usage: wikimedia category [flags] CATEGORY"); set.PrintDefaults() }
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	if pages <= 0 || limit < 1 || limit > 500 || thumbnailWidth <= 0 {
		fmt.Fprintln(stderr, "--pages and --thumbnail-width must be positive; --limit must be between 1 and 500")
		return 2
	}
	category, ok := requireOneReference(set, stderr)
	if !ok {
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	files, err := client.Commons().CollectCategoryFiles(ctx, category, pages,
		commons.CategoryLimit(limit), commons.CategoryThumbnailWidth(thumbnailWidth))
	if err != nil {
		return printError(stderr, err)
	}
	if err := writeOutput(output, files, common.format, common.compact, stdout); err != nil {
		return printError(stderr, err)
	}
	return 0
}

type nearbyRequest struct {
	coordinates []float64
	radius      float64
	limit       int
	boundingBox bool
}

func parseNearbyRequest(set *flag.FlagSet, bbox string, radius float64, limit int) (nearbyRequest, error) {
	if limit < 1 || limit > 500 {
		return nearbyRequest{}, fmt.Errorf("--limit must be between 1 and 500")
	}
	if strings.TrimSpace(bbox) != "" {
		if set.NArg() != 0 {
			return nearbyRequest{}, fmt.Errorf("use either coordinates or --bbox")
		}
		coordinates, err := parseFloatList(bbox, 4)
		if err != nil {
			return nearbyRequest{}, fmt.Errorf("invalid --bbox: %w", err)
		}
		return nearbyRequest{coordinates: coordinates, limit: limit, boundingBox: true}, nil
	}
	if set.NArg() != 2 {
		return nearbyRequest{}, fmt.Errorf("want LATITUDE and LONGITUDE, or --bbox")
	}
	if math.IsNaN(radius) || math.IsInf(radius, 0) || radius <= 0 || radius > 1000 {
		return nearbyRequest{}, fmt.Errorf("--radius must be between 0 and 1000 km")
	}
	coordinates, err := parseFloatList(strings.Join(set.Args(), ","), 2)
	if err != nil {
		return nearbyRequest{}, fmt.Errorf("invalid coordinates: %w", err)
	}
	return nearbyRequest{coordinates: coordinates, radius: radius, limit: limit}, nil
}

func parseFloatList(value string, want int) ([]float64, error) {
	parts := strings.Split(value, ",")
	if len(parts) != want {
		return nil, fmt.Errorf("want %d comma-separated numbers", want)
	}
	result := make([]float64, want)
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil, err
		}
		result[index] = parsed
	}
	return result, nil
}

func readSmallFile(path string, limit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(contents)) > limit {
		return "", fmt.Errorf("query file exceeds %d byte limit", limit)
	}
	return string(contents), nil
}

func requireOneReference(set *flag.FlagSet, stderr io.Writer) (string, bool) {
	references, ok := requireReferences(set, stderr)
	if !ok || len(references) != 1 {
		if ok {
			set.Usage()
		}
		return "", false
	}
	return references[0], true
}

func requireReferences(set *flag.FlagSet, stderr io.Writer) ([]string, bool) {
	if set.NArg() == 0 {
		set.Usage()
		return nil, false
	}
	references := make([]string, 0, set.NArg())
	for _, value := range set.Args() {
		reference := strings.TrimSpace(value)
		if reference == "" {
			fmt.Fprintln(stderr, "reference must not be empty")
			return nil, false
		}
		references = append(references, reference)
	}
	return references, true
}
func commandContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}
func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	result := parts[:0]
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
func writeOutput(path string, value any, format string, compact bool, stdout io.Writer) error {
	var buffer strings.Builder
	format = strings.ToLower(strings.TrimSpace(format))
	switch format {
	case "", "json":
		encoder := json.NewEncoder(&buffer)
		if !compact {
			encoder.SetIndent("", "  ")
		}
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(value); err != nil {
			return err
		}
	case "jsonl":
		if err := writeJSONLines(&buffer, value); err != nil {
			return err
		}
	case "text":
		buffer.WriteString(textOutput(value))
	default:
		return fmt.Errorf("unsupported output format %q (want json, jsonl, or text)", format)
	}
	if path == "" {
		_, err := io.WriteString(stdout, buffer.String())
		return err
	}
	return writeOutputFile(path, []byte(buffer.String()))
}

func writeJSONLines(writer io.Writer, value any) error {
	v := reflect.ValueOf(value)
	for v.IsValid() && (v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer) {
		if v.IsNil() {
			break
		}
		v = v.Elem()
	}
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if !v.IsValid() || (v.Kind() != reflect.Slice && v.Kind() != reflect.Array) {
		return encoder.Encode(value)
	}
	for index := 0; index < v.Len(); index++ {
		if err := encoder.Encode(v.Index(index).Interface()); err != nil {
			return err
		}
	}
	return nil
}

func textOutput(value any) string {
	var buffer strings.Builder
	switch value := value.(type) {
	case *wikimedia.Result:
		fmt.Fprintf(&buffer, "%s\t%s\n", value.ID, value.Label)
		if value.Description != "" {
			fmt.Fprintln(&buffer, value.Description)
		}
		fmt.Fprintf(&buffer, "media: %d\twarnings: %d\n", len(value.Media), len(value.Warnings))
	case []wikimedia.FetchManyItem:
		for _, item := range value {
			if item.Result != nil {
				fmt.Fprintf(&buffer, "%s\t%s\t%s\n", item.Reference, item.Result.ID, item.Result.Label)
			} else {
				fmt.Fprintf(&buffer, "%s\tERROR\t%s\n", item.Reference, item.Error)
			}
		}
	case []wikidata.SearchResult:
		for _, item := range value {
			fmt.Fprintf(&buffer, "%s\t%s\t%s\n", item.ID, item.Label, item.Description)
		}
	case []wikimedia.GeoItem:
		for _, item := range value {
			distance := ""
			if item.DistanceKM != nil {
				distance = strconv.FormatFloat(*item.DistanceKM, 'f', -1, 64)
			}
			fmt.Fprintf(&buffer, "%s\t%s\t%s\t%s\n", item.ID, item.Label, distance, item.Description)
		}
	case []commons.File:
		for _, file := range value {
			fmt.Fprintf(&buffer, "%s\t%s\t%s\n", file.Title, file.MIMEType, file.PageURL)
		}
	case *wikidata.SPARQLResult:
		fmt.Fprintln(&buffer, strings.Join(value.Variables, "\t"))
		for _, row := range value.Bindings {
			fields := make([]string, len(value.Variables))
			for index, variable := range value.Variables {
				fields[index] = row[variable].Value
			}
			fmt.Fprintln(&buffer, strings.Join(fields, "\t"))
		}
	case mediaOutput:
		fmt.Fprintf(&buffer, "%s\t%s\nmedia: %d\twarnings: %d\n", value.ID, value.Label, len(value.Media), len(value.Warnings))
	default:
		fmt.Fprintln(&buffer, value)
	}
	return buffer.String()
}

func writeOutputFile(path string, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".wikimedia-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	if _, err := temporary.Write(contents); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	committed = true
	return nil
}
func writeJSONFile(path string, value any, compact bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".wikimedia-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	encoder := json.NewEncoder(temporary)
	if !compact {
		encoder.SetIndent("", "  ")
	}
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	committed = true
	return nil
}
func printError(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "error:", err)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return 1
	}
	return 1
}
func flagExitCode(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	return 2
}
func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, `usage: wikimedia COMMAND [flags]

Commands:
  get       fetch a normalized Wikidata/Wikimedia aggregate
  media     fetch media metadata only
  download  fetch metadata and download selected media
  search    search Wikidata items by label or alias
  sparql    run a bounded Wikidata SPARQL query
  nearby    find Wikidata items around a point or in a bounding box
  category  list direct files in a Commons category
  version   print the version

Run "wikimedia COMMAND -h" for command-specific flags.`)
}
