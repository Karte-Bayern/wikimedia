# Karte Bayern Wikimedia

`github.com/karte-bayern/wikimedia` is an unofficial, read-only Go client and enrichment library for Wikidata, Wikimedia Commons, and Wikipedia. Given a Wikidata item ID, it returns a normalized entity, generic statements, coordinates, links, Commons media, license and attribution metadata, and optionally direct Commons category files and Wikipedia extracts.

The package is intended for maps, POI pipelines, cultural-data applications, importers, archives, and other services that need Wikimedia data without coupling their domain model to raw API envelopes.

Status: **v0.1.0 / pre-v1 API**. The module has no third-party Go dependencies and declares Go 1.22.

## Features

- Fetch one Wikidata `Q` item through `wbgetentities`, including item-redirect information.
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
