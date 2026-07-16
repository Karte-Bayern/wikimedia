package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karte-bayern/wikimedia"
	"github.com/karte-bayern/wikimedia/cache"
	"github.com/karte-bayern/wikimedia/download"
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
	languages, userAgent, cacheDir string
	timeout                        time.Duration
	compact                        bool
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
	set.StringVar(&values.userAgent, "user-agent", defaultApplicationUserAgent(), "descriptive User-Agent with contact information")
	set.StringVar(&values.cacheDir, "cache-dir", "", "optional persistent API cache directory")
	set.DurationVar(&values.timeout, "timeout", defaultTimeout, "overall command timeout")
	if compact {
		set.BoolVar(&values.compact, "compact", false, "emit compact JSON")
	}
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
	var output string
	addCommonFlags(set, &common, true, 45*time.Second)
	addFetchFlags(set, &fetch, true)
	set.StringVar(&output, "output", "", "write JSON to file instead of stdout")
	set.Usage = func() { fmt.Fprintln(set.Output(), "usage: wikimedia get [flags] QID"); set.PrintDefaults() }
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	id, ok := requireOneID(set, stderr)
	if !ok {
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	result, err := client.Fetch(ctx, id, fetch.options()...)
	if err != nil {
		return printError(stderr, err)
	}
	if err := writeJSON(output, result, common.compact, stdout); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func runMedia(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia media", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var fetch fetchFlags
	var output string
	addCommonFlags(set, &common, true, 45*time.Second)
	addFetchFlags(set, &fetch, false)
	set.StringVar(&output, "output", "", "write JSON to file instead of stdout")
	set.Usage = func() { fmt.Fprintln(set.Output(), "usage: wikimedia media [flags] QID"); set.PrintDefaults() }
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	id, ok := requireOneID(set, stderr)
	if !ok {
		return 2
	}
	ctx, cancel := commandContext(parent, common.timeout)
	defer cancel()
	client, err := common.newClient()
	if err != nil {
		return printError(stderr, err)
	}
	result, err := client.Fetch(ctx, id, fetch.options()...)
	if err != nil {
		return printError(stderr, err)
	}
	payload := struct {
		ID       string              `json:"id"`
		Label    string              `json:"label,omitempty"`
		Media    []wikimedia.Media   `json:"media"`
		Warnings []wikimedia.Warning `json:"warnings,omitempty"`
	}{result.ID, result.Label, result.Media, result.Warnings}
	if err := writeJSON(output, payload, common.compact, stdout); err != nil {
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

func runDownload(parent context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("wikimedia download", flag.ContinueOnError)
	set.SetOutput(stderr)
	var common commonFlags
	var fetch fetchFlags
	var outputDir, manifestPath, variant, kind string
	var overwrite, primaryOnly bool
	var maximumBytes int64
	addCommonFlags(set, &common, false, 5*time.Minute)
	addFetchFlags(set, &fetch, false)
	set.StringVar(&outputDir, "output", ".", "base output directory")
	set.StringVar(&manifestPath, "manifest", "", "manifest path (default: OUTPUT/QID/manifest.json)")
	set.StringVar(&variant, "variant", "thumbnail", "download variant: thumbnail or original")
	set.StringVar(&kind, "kind", "image", "media filter: image or all")
	set.BoolVar(&primaryOnly, "primary", false, "download only selected primary media")
	set.BoolVar(&overwrite, "overwrite", false, "replace existing regular files")
	set.Int64Var(&maximumBytes, "max-bytes", 50<<20, "maximum bytes per file")
	set.Usage = func() { fmt.Fprintln(set.Output(), "usage: wikimedia download [flags] QID"); set.PrintDefaults() }
	if err := set.Parse(args); err != nil {
		return flagExitCode(err)
	}
	id, ok := requireOneID(set, stderr)
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
	result, err := client.Fetch(ctx, id, fetch.options()...)
	if err != nil {
		return printError(stderr, err)
	}
	items := filterDownloadMedia(result.Media, primaryOnly, kind)
	if len(items) == 0 {
		fmt.Fprintln(stderr, "no matching media found")
		return 1
	}
	destination := filepath.Join(outputDir, id)
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return printError(stderr, err)
	}
	downloader, err := download.New(download.WithUserAgent(common.userAgent+" karte-bayern-wikimedia/"+wikimedia.Version), download.WithMaximumBytes(maximumBytes), download.WithOverwrite(overwrite))
	if err != nil {
		return printError(stderr, err)
	}
	manifestValue := manifest{EntityID: id, Label: result.Label, Variant: variant, FetchedAt: time.Now().UTC(), APIWarnings: result.Warnings}
	failed := false
	for _, media := range items {
		source, sourceErr := downloadSource(media, variant)
		if sourceErr != nil {
			manifestValue.Warnings = append(manifestValue.Warnings, sourceErr.Error())
			failed = true
			continue
		}
		local, fileErr := downloader.Download(ctx, source, destination)
		if fileErr != nil {
			manifestValue.Warnings = append(manifestValue.Warnings, media.Title+": "+fileErr.Error())
			failed = true
			continue
		}
		manifestValue.Files = append(manifestValue.Files, manifestFile{Local: *local, CommonsTitle: media.Title, CommonsPageURL: media.PageURL, OriginalURL: media.OriginalURL, ThumbnailURL: media.ThumbnailURL, Artist: media.Artist, Credit: media.Credit, Attribution: media.Attribution, License: media.License, LicenseShortName: media.LicenseShortName, LicenseURL: media.LicenseURL})
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

func requireOneID(set *flag.FlagSet, stderr io.Writer) (string, bool) {
	if set.NArg() != 1 {
		set.Usage()
		return "", false
	}
	id := strings.TrimSpace(set.Arg(0))
	if !strings.HasPrefix(id, "Q") {
		fmt.Fprintln(stderr, "QID must be a Wikidata item such as Q82425")
		return "", false
	}
	return id, true
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
func writeJSON(path string, value any, compact bool, stdout io.Writer) error {
	if path != "" {
		return writeJSONFile(path, value, compact)
	}
	encoder := json.NewEncoder(stdout)
	if !compact {
		encoder.SetIndent("", "  ")
	}
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
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
  version   print the version

Run "wikimedia COMMAND -h" for command-specific flags.`)
}
