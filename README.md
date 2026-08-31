# Karte Bayern Wikimedia

`github.com/karte-bayern/wikimedia` is an unofficial, read-only Go client and enrichment library for Wikidata, Wikimedia Commons, and Wikipedia. Given a Wikidata item ID, it returns a normalized entity, generic statements, coordinates, links, Commons media, license and attribution metadata, and optionally direct Commons category files and Wikipedia extracts.

The package is intended for maps, POI pipelines, cultural-data applications, importers, archives, and other services that need Wikimedia data without coupling their domain model to raw API envelopes.

Status: **v0.1.0 / pre-v1 API**. The module has no third-party Go dependencies and declares Go 1.22.

## Features

- Fetch one Wikidata `Q` item through `wbgetentities`, including item-redirect information.
- Resolve Wikidata, Wikipedia, and OpenStreetMap IDs and common page URLs.
- Resolve Commons file and category titles or URLs without an intervening item.
- Search Wikidata items by label, alias, or other indexed text.
- Batch direct Q-ID enrichment while retaining per-reference failures.
- Preserve claims, qualifiers, references, ranks, and unknown raw data values.
- Select labels, descriptions, and aliases using an explicit language fallback.
- Normalize coordinates, sitelinks, official websites, and Commons references.
- Resolve direct Commons media in batches, including `P18`, banners, maps, logos, coats of arms, flags, signatures, video, and audio.
- Follow MediaWiki title normalization and file-page redirects while preserving requested titles as aliases.
- Return original and thumbnail URLs, dimensions, MIME/media type, uploader, SHA-1, description, artist, credit, attribution, license, usage terms, and restrictions.
- Read direct files from linked Commons categories with explicit page and media limits and no implicit recursion.
- Optionally fetch introductory plain-text extracts from preferred Wikipedias.
- Rank and deduplicate media while retaining every discovery source.
- Return useful partial results with structured warnings when optional services fail.
- Respect `maxlag`, HTTP 429 and `Retry-After`, bounded retries, contexts, and response-size limits.
- Cache raw API responses in memory, on a filesystem, or through an application-provided cache.
- Download media atomically with URL restrictions, byte limits, safe names, and MediaWiki base-36 SHA-1 verification.
- Download media batches concurrently with duplicate detection, progress events, and resumable manifests.
- Use the same public API through the included CLI.

## Installation

```bash
go get github.com/karte-bayern/wikimedia
```

## Basic use

A descriptive User-Agent with a contact address or project URL is mandatory:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/karte-bayern/wikimedia"
)

func main() {
    client, err := wikimedia.New(
        wikimedia.WithUserAgent("my-map/1.0 (admin@example.org)"),
        wikimedia.WithLanguages("de", "en"),
    )
    if err != nil {
        log.Fatal(err)
    }

    result, err := client.Fetch(context.Background(), "Q82425")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("%s: %s\n", result.Label, result.Description)
    if image := result.PrimaryMedia(); image != nil {
        fmt.Println(image.ThumbnailURL)
        fmt.Printf("%s · %s · %s\n",
            image.Attribution,
            image.LicenseShortName,
            image.PageURL,
        )
    }
}
```

`Q82425` is the Wikidata item for the Brandenburg Gate. The previously discussed example `Q82489` is a different item; the package accepts any valid non-zero `Q` item ID.

## Resolve OSM and Wikipedia references

In addition to a Wikidata `Q` ID, the aggregate client resolves a Wikipedia
page or a typed OpenStreetMap object ID to its linked Wikidata item. OSM IDs
are only unique within their object type:

```go
fromWikipedia, err := client.FetchByWikipedia(ctx, "de", "Brandenburger Tor")
fromOSM, err := client.FetchByOSM(ctx, wikimedia.OSMRelation, "62422")
```

`FetchByOSM` supports `OSMRelation`, `OSMWay`, and `OSMNode`, mapped to the
Wikidata properties P402, P10689, and P11693 respectively. The CLI exposes
the same resolution:

```bash
wikimedia get --wiki 'de:Brandenburger Tor'
wikimedia get --osm relation/62422
```

Any `get`, `media`, or `download` positional argument can instead be a
Wikidata, Wikipedia, or OpenStreetMap URL. Compact references are also useful
in configuration files and scripts:

```bash
wikimedia get https://de.wikipedia.org/wiki/Brandenburger_Tor
wikimedia media https://www.openstreetmap.org/relation/62422
wikimedia get wikidata:Q82425
wikimedia get osm:relation:62422
wikimedia get https://commons.wikimedia.org/wiki/File:Brandenburger_Tor_morgens.jpg
wikimedia media 'Category:Brandenburg Gate'
```

## Batch enrichment and output formats

`FetchMany` preserves input order, batches direct Q IDs, and keeps each
failure alongside successful results:

```go
items, err := client.FetchMany(ctx, []string{
    "Q64",
    "https://de.wikipedia.org/wiki/Brandenburger_Tor",
    "relation/62422",
})
for _, item := range items {
    if item.Err != nil {
        log.Printf("%s: %v", item.Reference, item.Err)
        continue
    }
    fmt.Println(item.Result.ID, item.Result.Label)
}
```

The CLI accepts multiple references with `get`; `json`, `jsonl`, and
tab-friendly `text` output are available on data commands. Common flags have
short aliases, such as `-l` for `--languages`, `-t` for `--timeout`, and `-o`
for `--output`.

```bash
wikimedia get --format jsonl Q64 Q90 relation/62422
wikimedia search -l en -F text 'Brandenburg Gate'
```

## Item search

Use full-text search for labels and aliases, then fetch a selected item. It is
separate from SPARQL, which is intended for structured graph queries.

```go
matches, err := client.SearchItems(ctx, "Brandenburg Gate",
    wikidata.SearchLanguage("en"),
    wikidata.SearchLimit(5),
)
```

The CLI emits the same structured results:

```bash
wikimedia search --language en --limit 5 'Brandenburg Gate'
```

By default, `Fetch` loads the Wikidata entity, claims, Commons references, and direct media. Fan-out operations are opt-in:

```go
result, err := client.Fetch(
    ctx,
    "Q82425",
    wikimedia.WithCommonsCategories(true),
    wikimedia.WithWikipediaSummaries(true),
    wikimedia.WithMediaLimit(20),
    wikimedia.WithCategoryPageLimit(2),
    wikimedia.WithThumbnailWidth(1200),
)
```

For a direct Wikipedia read, `Article` contains a normalized lead extract,
the page-image title, a display-sized thumbnail, and original-image metadata.
Use a sentence limit when a compact teaser is sufficient:

```go
reader, err := wikipedia.NewClient(
    wikipedia.WithUserAgent("my-map/1.0 (admin@example.org)"),
    wikipedia.WithExtractSentences(3),
)
article, err := reader.GetSummary(ctx, "de", "Brandenburger Tor")
fmt.Println(article.Extract)
if article.Thumbnail != nil {
    fmt.Println(article.Thumbnail.Source) // display image
}
if article.Original != nil {
    fmt.Println(article.Original.Source) // original-file URL; choose deliberately when downloading it
}
```

## Public package structure

The root package is the convenience API. Service-specific clients remain independently usable:

```text
wikimedia          aggregate, normalization, ranking, partial results
mediawiki          generic Action API transport
wikidata           typed wbgetentities client and generic claims
commons            file metadata and explicit category pagination
wikipedia          language-specific introductory extracts
cache              memory and atomic filesystem cache
download           bounded atomic media downloads
cmd/wikimedia      command-line client
```

The project deliberately contains no Karte.Bayern-specific POI, OSM, database, queue, image-proxy, or frontend type.

## SPARQL

For graph-shaped or multi-item queries, use the bounded Wikidata Query Service
client exposed by the aggregate client. It sends `POST` requests, requires the
same descriptive User-Agent, respects contexts, retries rate limits and
temporary 5xx responses with the configured bounded retry policy, and decodes
standard SPARQL Results JSON. Keep queries selective and paginated; SPARQL is
not intended for fuzzy text search or large-scale exports.

```go
result, err := client.SPARQL().Query(ctx, `
    SELECT ?item WHERE {
      ?item wdt:P31 wd:Q515 .
    }
    LIMIT 10
`)
for _, row := range result.Bindings {
    fmt.Println(row["item"].Value)
}
```

Run a query directly from the CLI with inline text or a `.rq` file:

```bash
wikimedia sparql --file nearby-museums.rq
```

For caller-controlled, safe pagination, provide a query builder that applies
the supplied `OFFSET` and `LIMIT` values:

```go
err := client.SPARQL().QueryPages(ctx, 100, 10,
    func(offset, limit int) string {
        return fmt.Sprintf("SELECT ?item WHERE { ?item wdt:P31 wd:Q515 } LIMIT %d OFFSET %d", limit, offset)
    },
    func(page *wikidata.SPARQLResult) error {
        // process page.Bindings
        return nil
    },
)
```

## Geographic queries

Use the bounded helpers for nearby or bounding-box lookups. Coordinates are
WGS84 and distances are kilometres; both helpers limit results to 500 items.
The CLI exposes the same bounded queries and accepts JSON, JSONL, or
tab-separated text output.

```go
nearby, err := client.FindNearby(ctx, 52.5163, 13.3777, 2, 25)
inBox, err := client.FindInBoundingBox(ctx, 52.4, 13.2, 52.6, 13.6, 100)
```

```bash
# Radius in kilometres around longitude/latitude.
wikimedia nearby 52.5163 13.3777 --radius 2 --limit 25 -F text

# Bounding box: south,west,north,east.
wikimedia nearby --bbox 52.50,13.35,52.55,13.45 -F jsonl
```

## Typed convenience accessors

When claims are included (the default), `Result` offers helpers for common
application fields while retaining the original generic claims:

```go
inception, ok := result.Inception()          // P571
sites := result.OfficialWebsites()            // P856
areas := result.AdministrativeAreas()         // P131
designations := result.HeritageDesignations() // P1435
hours := result.OpeningHours()                // P8629
address := result.Address()                   // P6375, P670, P281, P17
```

For custom properties, the lower-level Wikidata types preserve statements,
qualifiers, and references. They expose deterministic rank handling without
requiring callers to decode raw JSON again:

```go
entity, err := client.Wikidata().GetEntity(ctx, "Q82425")
claim, ok := entity.PreferredClaim("P18", false)
if ok {
    for _, source := range claim.ReferenceSnaks("P854") { // reference URL
        fmt.Println(source.ValueString())
    }
}
```

## Pagination and bulk downloads

Commons pagination is available as a bounded callback or a collecting helper:

```go
files, err := client.Commons().CollectCategoryFiles(
    ctx, "Category:Brandenburg Gate", 3,
    commons.CategoryLimit(50),
)
```

```bash
# Direct files only; subcategories are never traversed implicitly.
wikimedia category 'Category:Brandenburg Gate' --pages 2 --limit 100 -F text
```

For many media representations, the downloader limits concurrency, emits
serialized progress events, skips duplicate URLs, and can resume successful
items from an atomic manifest:

```go
batch, err := downloader.DownloadBatch(ctx, sources, "images",
    download.WithConcurrency(4),
    download.WithManifest("images/downloads.json"),
    download.WithResume(true),
)
```

The CLI `download` command uses this batch mechanism. `--concurrency` bounds
parallel downloads and `--resume` reuses the prior `.wikimedia-downloads.json`
manifest in the entity output directory.

## Result model

Important fields include:

```go
type Result struct {
    ID           string
    EntityURL    string
    Label        string
    Description  string
    Labels       map[string]string
    Coordinates  *wikidata.CoordinateValue
    Claims       map[string][]wikidata.Claim
    Sitelinks    map[string]wikidata.Sitelink
    Commons      []wikimedia.CommonsReference
    Media        []wikimedia.Media
    Articles     []wikipedia.Article
    Links        []wikimedia.Link
    Warnings     []wikimedia.Warning
}
```

Claims remain generic. Helpers on `wikidata.Snak` decode strings, linked entities, coordinates, times, quantities, and monolingual text without discarding the original JSON value.

## Media discovery and ranking

The default media registry is:

| Property | Role |
|---|---|
| `P18` | image |
| `P948` | page banner |
| `P242` | map |
| `P154` | logo |
| `P94` | coat of arms |
| `P41` | flag |
| `P109` | signature |
| `P10` | video |
| `P51` | audio |

Replace or extend it per request:

```go
wikimedia.WithMediaProperties(
    wikimedia.MediaProperty{
        ID: "P18", Kind: wikimedia.MediaKindImage, BaseScore: 1000,
    },
)

wikimedia.WithAdditionalMediaProperties(
    wikimedia.MediaProperty{
        ID: "P12345", Kind: wikimedia.MediaKindImage, BaseScore: 600,
    },
)
```

Ranking favors direct `P18`, preferred-rank statements, useful resolution, and complete license/attribution metadata. Logos, flags, audio, video, tiny files, and extreme aspect ratios receive penalties for primary-image selection. This is a presentation heuristic, not a copyright or quality judgment.

A file found through both a claim and a category is deduplicated by Commons page ID, SHA-1, or normalized title. Every reason for inclusion remains in `Media.Sources`.

## Commons metadata

The low-level Commons API is explicit:

```go
cm, err := commons.NewClient(
    commons.WithUserAgent("my-importer/1.0 (https://example.org/contact)"),
    commons.WithLanguage("de"),
)
file, err := cm.GetFile(ctx, "Brandenburger Tor morgens.jpg")
page, err := cm.ListCategoryFiles(
    ctx,
    "Category:Brandenburg Gate",
    commons.CategoryLimit(50),
)
if err != nil {
    log.Fatal(err)
}

for page.ContinueToken != "" {
    page, err = cm.ListCategoryFiles(
        ctx,
        "Category:Brandenburg Gate",
        commons.CategoryLimit(50),
        commons.CategoryContinueWith(
            page.ContinueToken,
            page.ContinueValue,
        ),
    )
    if err != nil {
        log.Fatal(err)
    }
}
```

The client requests only selected `extmetadata` fields by default. The larger `commonmetadata` block is opt-in:

```go
file, err = cm.GetFile(
    ctx,
    "Brandenburger Tor morgens.jpg",
    commons.FileCommonMetadata(true),
)
```

`extmetadata` values can contain HTML. Display fields are converted to conservative plain text; the selected upstream values remain available in `ExtendedMetadata`. Do not render upstream HTML without an application-specific sanitizer.

## Partial results

Wikidata is the required root resource. A Wikidata failure returns an error. Commons and Wikipedia are enrichments; recoverable failures become `Result.Warnings` while successful entity data remains available. Context cancellation and deadline expiration remain fatal.

## Caching

```go
filesystemCache, err := cache.NewFilesystem("./var/wikimedia-cache")
if err != nil {
    log.Fatal(err)
}

client, err := wikimedia.New(
    wikimedia.WithUserAgent("my-map/1.0 (admin@example.org)"),
    wikimedia.WithCache(filesystemCache, wikimedia.CacheTTLs{
        Wikidata:  12 * time.Hour,
        Commons:   7 * 24 * time.Hour,
        Wikipedia: 12 * time.Hour,
    }),
)
```

The defaults are 24 hours for Wikidata, seven days for Commons, and 24 hours for Wikipedia. `cache.Memory` is useful for short processes. `cache.Filesystem` hashes canonical request URLs and commits files atomically. Applications can implement the small `mediawiki.Cache` interface for Redis, SQL, or an existing cache.

## Downloads

Metadata fetching has no filesystem side effects. Downloads are a separate package:

```go
downloader, err := download.New(
    download.WithUserAgent("my-map/1.0 (admin@example.org)"),
    download.WithMaximumBytes(50<<20),
)

local, err := downloader.Download(ctx, download.Source{
    URL:          media.OriginalURL,
    FileName:     media.FileName,
    MIMEType:     media.MIMEType,
    Size:         media.Size,
    SHA1Base36:   media.SHA1,
    CommonsTitle: media.Title,
}, "./media")
```

By default, downloads allow only HTTPS URLs on `upload.wikimedia.org`. Alternate installations must explicitly replace the allow-list. Requests use an identity transfer representation so size and SHA-1 checks apply to stored bytes. The downloader enforces advertised and observed size limits, validates the final redirected URL, rejects unsafe paths and symlink replacement, writes to a temporary file, and commits without silently clobbering an existing destination.

Thumbnail names should be derived from the thumbnail URL because Commons can rasterize an SVG into a PNG or JPEG representation. The CLI does this automatically.

SHA-1 is used only as an upstream compatibility integrity check, not as a modern authenticity mechanism.

## CLI

Install:

```bash
go install github.com/karte-bayern/wikimedia/cmd/wikimedia@latest
```

Examples:

```bash
wikimedia get Q82425
wikimedia get --categories --wikipedia --media-limit 12 Q82425
wikimedia media --categories Q82425
wikimedia download --variant thumbnail --primary --output ./data Q82425
wikimedia download --variant original --media-limit 5 --output ./data Q82425
```

Set the application identity through `--user-agent` or `WIKIMEDIA_USER_AGENT`. The `download` command writes a `manifest.json` containing local checksums, Commons page URLs, artist/credit fields, and license metadata.

## Operational behavior

- All network operations accept `context.Context`.
- Reads use GET and set `format=json`, `formatversion=2`, and `maxlag=5` by default.
- HTTP 429, Action API `maxlag`, and temporary 5xx responses are retried with bounded exponential backoff; `Retry-After` takes precedence.
- Response bodies are bounded before JSON decoding.
- Direct Commons file titles are batched. Normalization and redirects are mapped back to the originally requested aliases.
- Category members and `imageinfo` are combined in one generator request.
- There is no implicit recursive category traversal and no SPARQL query in the single-item path.

See [docs/design.md](docs/design.md) for rationale and upstream references.

## Licensing and attribution

The Go source code in this repository is MIT licensed. Data and media returned by Wikimedia services are not covered by the repository license.

Treat `Media.Artist`, `Credit`, `Attribution`, `LicenseShortName`, `LicenseURL`, `UsageTerms`, `Restrictions`, and `PageURL` as source metadata, not a legal determination. Preserve the Commons file-page link and verify the specific license before republication. See [docs/licensing.md](docs/licensing.md).

## Development

```bash
go fmt ./...
go vet ./...
go test -race ./...
go build ./cmd/wikimedia
```

Unit tests use deterministic fixtures and `httptest.Server`; they do not depend on live Wikimedia availability.

The project is not affiliated with or endorsed by the Wikimedia Foundation.
