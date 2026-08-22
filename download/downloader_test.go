package download

import (
	"context"
	"crypto/sha1" // #nosec G505 -- test fixture for Wikimedia-compatible checksum.
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func TestDownloadVerifiesAndCommits(t *testing.T) {
	payload := []byte("image-bytes")
	sum := sha1.Sum(payload)
	expected := digestBase36(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("encoding=%q", r.Header.Get("Accept-Encoding"))
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	downloader, err := New(WithUserAgent("download-test/1.0 (test@example.org)"), WithAllowedHosts(parsed.Hostname()), WithAllowedSchemes("http"), WithStrictMIME(true))
	if err != nil {
		t.Fatal(err)
	}
	file, err := downloader.Download(context.Background(), Source{URL: server.URL + "/photo.jpg", FileName: "folder/photo.jpg", MIMEType: "image/jpeg", Size: int64(len(payload)), SHA1Base36: expected, CommonsTitle: "File:Photo.jpg"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if file.FileName != "photo.jpg" || file.SHA1Base36 != expected || file.Size != int64(len(payload)) {
		t.Fatalf("file=%+v", file)
	}
	stored, err := os.ReadFile(file.Path)
	if err != nil || string(stored) != string(payload) {
		t.Fatalf("stored=%q err=%v", stored, err)
	}
}

func TestDownloadRejectsEncodedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write([]byte("encoded"))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	downloader, err := New(WithUserAgent("download-test/1.0 (test@example.org)"), WithAllowedHosts(parsed.Hostname()), WithAllowedSchemes("http"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = downloader.Download(context.Background(), Source{URL: server.URL + "/x.jpg"}, t.TempDir())
	if !errors.Is(err, ErrContentEncoding) {
		t.Fatalf("err=%v", err)
	}
}

func TestDownloadDoesNotClobber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("new")) }))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.jpg"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloader, err := New(WithUserAgent("download-test/1.0 (test@example.org)"), WithAllowedHosts(parsed.Hostname()), WithAllowedSchemes("http"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = downloader.Download(context.Background(), Source{URL: server.URL + "/x.jpg", FileName: "x.jpg"}, dir)
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err=%v", err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "x.jpg"))
	if string(content) != "old" {
		t.Fatalf("content=%q", content)
	}
}

func TestFileNameFromURLUsesActualRepresentation(t *testing.T) {
	parsed, _ := url.Parse("https://upload.wikimedia.org/wikipedia/commons/thumb/a/a0/Logo.svg/1200px-Logo.svg.png")
	if got := FileNameFromURL(parsed); got != "1200px-Logo.svg.png" {
		t.Fatalf("got=%q", got)
	}
	if got := SanitizeFileName(`../bad:<name>.jpg`); strings.ContainsAny(got, `/:<>`) {
		t.Fatalf("unsafe=%q", got)
	}
	if got := SanitizeFileName("CON.txt"); got != "_CON.txt" {
		t.Fatalf("reserved=%q", got)
	}
	long := strings.Repeat("ä", 200) + ".jpg"
	if got := SanitizeFileName(long); len(got) > 240 || !strings.HasSuffix(got, ".jpg") || !utf8.ValidString(got) {
		t.Fatalf("long filename invalid: bytes=%d value=%q", len(got), got)
	}
}

func TestDownloadBatchDeduplicatesAndResumes(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	downloader, err := New(WithUserAgent("download-test/1.0 (test@example.org)"), WithAllowedHosts(parsed.Hostname()), WithAllowedSchemes("http"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	manifest := filepath.Join(directory, "batch.json")
	sources := []Source{
		{URL: server.URL + "/one.jpg", FileName: "one.jpg", CommonsTitle: "File:One.jpg"},
		{URL: server.URL + "/one.jpg", FileName: "duplicate.jpg", CommonsTitle: "File:One.jpg"},
		{URL: server.URL + "/two.jpg", FileName: "two.jpg", CommonsTitle: "File:Two.jpg"},
	}
	result, err := downloader.DownloadBatch(context.Background(), sources, directory, WithConcurrency(2), WithManifest(manifest))
	if err != nil || result.Items[0].Status != BatchDownloaded || result.Items[1].Status != BatchDuplicate || result.Items[2].Status != BatchDownloaded || calls.Load() != 2 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls.Load(), err)
	}
	resumed, err := downloader.DownloadBatch(context.Background(), sources, directory, WithConcurrency(2), WithManifest(manifest), WithResume(true))
	if err != nil || resumed.Items[0].Status != BatchSkipped || resumed.Items[2].Status != BatchSkipped || calls.Load() != 2 {
		t.Fatalf("resumed=%+v calls=%d err=%v", resumed, calls.Load(), err)
	}
}
