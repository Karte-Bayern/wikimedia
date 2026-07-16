package main

import (
	"bytes"
	"context"
	"flag"
	"io"
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
	out.Reset()
	errOut.Reset()
	if code := run(context.Background(), []string{"media", "-h"}, &out, &errOut); code != 0 {
		t.Fatalf("code=%d", code)
	}
	help := errOut.String()
	if strings.Contains(help, "-wikipedia") || strings.Contains(help, "-claims") || strings.Contains(help, "-raw") {
		t.Fatalf("irrelevant flags in help:\n%s", help)
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

func TestRequireOneID(t *testing.T) {
	var stderr bytes.Buffer
	set := newTestFlagSet([]string{"Q82425"})
	id, ok := requireOneID(set, &stderr)
	if !ok || id != "Q82425" {
		t.Fatalf("id=%q ok=%v", id, ok)
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
