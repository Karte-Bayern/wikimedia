package main

import (
	"context"
	"fmt"
	"log"

	"github.com/karte-bayern/wikimedia/commons"
	"github.com/karte-bayern/wikimedia/wikidata"
)

func main() {
	ctx := context.Background()
	wd, err := wikidata.NewClient(
		wikidata.WithUserAgent("wikimedia-lowlevel-example/1.0 (admin@example.org)"),
		wikidata.WithLanguages("de", "en"),
	)
	if err != nil {
		log.Fatal(err)
	}
	entity, err := wd.GetEntity(ctx, "Q82425")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(entity.Labels["de"].Value)

	cm, err := commons.NewClient(
		commons.WithUserAgent("wikimedia-lowlevel-example/1.0 (admin@example.org)"),
		commons.WithLanguage("de"),
	)
	if err != nil {
		log.Fatal(err)
	}
	file, err := cm.GetFile(ctx, "Brandenburger Tor morgens.jpg")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(file.OriginalURL, file.LicenseShortName)
}
