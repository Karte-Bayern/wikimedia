package commons

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetFileNormalizesRedirectsAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if !strings.Contains(query.Get("iiprop"), "extmetadata") {
			t.Error("extmetadata missing")
		}
		if strings.Contains(query.Get("iiprop"), "commonmetadata") {
			t.Error("commonmetadata unexpectedly enabled")
		}
		_, _ = w.Write([]byte(`{"query":{"normalized":[{"from":"File:old name.jpg","to":"File:Old name.jpg"}],"redirects":[{"from":"File:Old name.jpg","to":"File:Target.jpg"}],"pages":[{"pageid":123,"ns":6,"title":"File:Target.jpg","imageinfo":[{"timestamp":"2025-01-02T03:04:05Z","user":"Photographer","size":12345,"width":2000,"height":1200,"sha1":"abc123","url":"https://upload.wikimedia.org/x/Target.jpg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Target.jpg","thumburl":"https://upload.wikimedia.org/x/1200px-Target.jpg","thumbwidth":1200,"thumbheight":720,"mime":"image/jpeg","mediatype":"BITMAP","extmetadata":{"ImageDescription":{"value":"<b>Gate</b> &amp; square","source":"commons-desc-page"},"Artist":{"value":"<a href='#'>Alice</a>"},"LicenseShortName":{"value":"CC BY-SA 4.0"},"LicenseUrl":{"value":"https://creativecommons.org/licenses/by-sa/4.0/"}}}]}]}}`))
	}))
	defer server.Close()
	client, err := NewClient(WithEndpoint(server.URL), WithUserAgent("commons-test/1.0 (test@example.org)"), WithLanguage("de"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := client.GetFile(context.Background(), "old_name.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if file.Title != "File:Target.jpg" || file.Description != "Gate & square" || file.Artist != "Alice" || file.LicenseShortName != "CC BY-SA 4.0" {
		t.Fatalf("file=%+v", file)
	}
	if !containsTitle(file.Aliases, "File:old name.jpg") || !containsTitle(file.Aliases, "File:Old name.jpg") {
		t.Fatalf("aliases=%v", file.Aliases)
	}
}

func TestGetFileCommonMetadataOptIn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("iiprop"), "commonmetadata") {
			t.Error("commonmetadata not enabled")
		}
		_, _ = w.Write([]byte(`{"query":{"pages":[{"pageid":1,"ns":6,"title":"File:A.jpg","imageinfo":[{"url":"https://upload.wikimedia.org/A.jpg","commonmetadata":{"CameraModel":"X"}}]}]}}`))
	}))
	defer server.Close()
	client, err := NewClient(WithEndpoint(server.URL), WithUserAgent("commons-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := client.GetFile(context.Background(), "A.jpg", FileCommonMetadata(true))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(file.CommonMetadata), "CameraModel") {
		t.Fatalf("common=%s", file.CommonMetadata)
	}
}

func TestListCategoryFilesAndContinue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("generator") != "categorymembers" || q.Get("gcmtype") != "file" {
			t.Fatalf("query=%v", q)
		}
		if q.Get("gcmcontinue") != "start" || q.Get("continue") != "-||" {
			t.Errorf("continue=%q", q.Get("gcmcontinue"))
		}
		_, _ = w.Write([]byte(`{"continue":{"gcmcontinue":"next","continue":"gcmcontinue||"},"query":{"pages":[{"pageid":2,"ns":6,"title":"File:B.jpg","imageinfo":[{"url":"https://upload.wikimedia.org/B.jpg","mime":"image/jpeg","mediatype":"BITMAP"}]},{"pageid":1,"ns":6,"title":"File:A.jpg","imageinfo":[{"url":"https://upload.wikimedia.org/A.jpg","mime":"image/jpeg","mediatype":"BITMAP"}]}]}}`))
	}))
	defer server.Close()
	client, err := NewClient(WithEndpoint(server.URL), WithUserAgent("commons-test/1.0 (test@example.org)"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ListCategoryFiles(context.Background(), "Example", CategoryLimit(2), CategoryContinue("start"))
	if err != nil {
		t.Fatal(err)
	}
	if page.Category != "Category:Example" || page.ContinueToken != "next" || page.ContinueValue != "gcmcontinue||" || len(page.Files) != 2 || page.Files[0].Title != "File:A.jpg" {
		t.Fatalf("page=%+v", page)
	}
}

func containsTitle(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
