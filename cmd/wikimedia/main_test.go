package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karte-bayern/wikimedia"
)

func TestVersionAndHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), wikimedia.Version) {
		t.Fatalf("stdout=%q", out.String())
	}
	for _, command := range []string{"get", "media", "search", "sparql", "nearby", "category"} {
		out.Reset()
		errOut.Reset()
		if code := run(context.Background(), []string{command, "-h"}, &out, &errOut); code != 0 {
			t.Fatalf("command=%s code=%d", command, code)
		}
		if command == "media" && (strings.Contains(errOut.String(), "-wikipedia") || strings.Contains(errOut.String(), "-claims") || strings.Contains(errOut.String(), "-raw")) {
			t.Fatalf("irrelevant flags in help:\n%s", errOut.String())
		}
	}
}

func TestDownloadSourceUsesThumbnailURLFileName(t *testing.T) {
	media := wikimedia.Media{Title: "File:Logo.svg", FileName: "Logo.svg", OriginalURL: "https://upload.wikimedia.org/Logo.svg", ThumbnailURL: "https://upload.wikimedia.org/thumb/a/a/Logo.svg/1200px-Logo.svg.png", MIMEType: "image/svg+xml", Size: 100, SHA1: "abc"}
	source, err := downloadSource(media, "thumbnail")
	if err != nil {
		t.Fatal(err)
	}
	if source.FileName != "1200px-Logo.svg.png" || source.Size != 0 || source.SHA1Base36 != "" || source.MIMEType != "" {
		t.Fatalf("source=%+v", source)
	}
	original, err := downloadSource(media, "original")
	if err != nil {
		t.Fatal(err)
	}
	if original.FileName != "Logo.svg" || original.Size != 100 || original.MIMEType != "image/svg+xml" {
		t.Fatalf("original=%+v", original)
	}
}

func TestRequireOneReference(t *testing.T) {
	var stderr bytes.Buffer
	set := newTestFlagSet([]string{"https://de.wikipedia.org/wiki/Brandenburger_Tor"})
	reference, ok := requireOneReference(set, &stderr)
	if !ok || reference != "https://de.wikipedia.org/wiki/Brandenburger_Tor" {
		t.Fatalf("reference=%q ok=%v", reference, ok)
	}
}

func newTestFlagSet(args []string) *flag.FlagSet {
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	_ = set.Parse(args)
	return set
}

func TestFilterDownloadMediaImageMeansVisualMedia(t *testing.T) {
	values := []wikimedia.Media{
		{Title: "File:Photo.jpg", Kind: wikimedia.MediaKindImage, MIMEType: "image/jpeg"},
		{Title: "File:Map.svg", Kind: wikimedia.MediaKindMap, MIMEType: "image/svg+xml"},
		{Title: "File:Document.pdf", Kind: wikimedia.MediaKindOther, MIMEType: "application/pdf"},
		{Title: "File:Sound.ogg", Kind: wikimedia.MediaKindAudio, MIMEType: "audio/ogg"},
	}
	filtered := filterDownloadMedia(values, false, "image")
	if len(filtered) != 2 || filtered[0].Title != "File:Photo.jpg" || filtered[1].Title != "File:Map.svg" {
		t.Fatalf("filtered=%+v", filtered)
	}
	all := filterDownloadMedia(values, false, "all")
	if len(all) != len(values) {
		t.Fatalf("all=%+v", all)
	}
}

func TestReadSmallFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.rq")
	if err := os.WriteFile(path, []byte("SELECT * WHERE {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	contents, err := readSmallFile(path, 1024)
	if err != nil || contents != "SELECT * WHERE {}" {
		t.Fatalf("contents=%q err=%v", contents, err)
	}
	if _, err := readSmallFile(path, 4); err == nil {
		t.Fatal("oversized file was accepted")
	}
}

func TestWriteOutputFormats(t *testing.T) {
	var output bytes.Buffer
	values := []wikimedia.SearchResult{{ID: "Q64", Label: "Berlin"}, {ID: "Q90", Label: "Paris"}}
	if err := writeOutput("", values, "jsonl", false, &output); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(output.String()), "\n"); len(lines) != 2 || !strings.Contains(lines[0], "Q64") {
		t.Fatalf("jsonl=%q", output.String())
	}
	output.Reset()
	if err := writeOutput("", values, "text", false, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Q64\tBerlin") {
		t.Fatalf("text=%q", output.String())
	}
	if err := writeOutput("", values, "yaml", false, &output); err == nil {
		t.Fatal("invalid format was accepted")
	}
}

func TestParseFloatList(t *testing.T) {
	values, err := parseFloatList("52.5, 13.4", 2)
	if err != nil || len(values) != 2 || values[0] != 52.5 || values[1] != 13.4 {
		t.Fatalf("values=%v err=%v", values, err)
	}
	if _, err := parseFloatList("52.5", 2); err == nil {
		t.Fatal("wrong number of coordinates was accepted")
	}
	if _, err := parseFloatList("north,13.4", 2); err == nil {
		t.Fatal("non-numeric coordinate was accepted")
	}
}

func TestParseNearbyRequest(t *testing.T) {
	point := newTestFlagSet([]string{"52.5", "13.4"})
	request, err := parseNearbyRequest(point, "", 2, 25)
	if err != nil || request.boundingBox || request.radius != 2 || len(request.coordinates) != 2 {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	box := newTestFlagSet(nil)
	request, err = parseNearbyRequest(box, "52.4,13.3,52.6,13.5", 0, 25)
	if err != nil || !request.boundingBox || len(request.coordinates) != 4 {
		t.Fatalf("request=%+v err=%v", request, err)
	}
	if _, err := parseNearbyRequest(point, "", 0, 25); err == nil {
		t.Fatal("invalid radius was accepted")
	}
	if _, err := parseNearbyRequest(point, "52.4,13.3,52.6,13.5", 2, 25); err == nil {
		t.Fatalf("err=%v", err)
	}
}

func TestNearbyAndCategoryRejectInvalidFlagsBeforeNetworkWork(t *testing.T) {
	for _, args := range [][]string{
		{"nearby", "--radius", "0", "52.5", "13.4"},
		{"nearby", "--limit", "501", "52.5", "13.4"},
		{"category", "--pages", "0", "Category:Example"},
		{"category", "--limit", "501", "Category:Example"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code != 2 {
			t.Fatalf("args=%v code=%d stderr=%s", args, code, stderr.String())
		}
	}
}
