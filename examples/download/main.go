package main

import (
	"context"
	"fmt"
	"log"

	"github.com/karte-bayern/wikimedia"
	"github.com/karte-bayern/wikimedia/download"
)

func main() {
	ctx := context.Background()
	client, err := wikimedia.New(
		wikimedia.WithUserAgent("wikimedia-download-example/1.0 (admin@example.org)"),
		wikimedia.WithLanguages("de", "en"),
	)
	if err != nil {
		log.Fatal(err)
	}
	result, err := client.Fetch(ctx, "Q82425", wikimedia.WithMediaLimit(1))
	if err != nil {
		log.Fatal(err)
	}
	media := result.PrimaryMedia()
	if media == nil {
		log.Fatal("no image")
	}

	downloader, err := download.New(
		download.WithUserAgent("wikimedia-download-example/1.0 (admin@example.org)"),
		download.WithMaximumBytes(50<<20),
	)
	if err != nil {
		log.Fatal(err)
	}
	file, err := downloader.Download(ctx, download.Source{
		URL: media.OriginalURL, FileName: media.FileName, MIMEType: media.MIMEType,
		Size: media.Size, SHA1Base36: media.SHA1, CommonsTitle: media.Title,
	}, "./media")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(file.Path)
}
