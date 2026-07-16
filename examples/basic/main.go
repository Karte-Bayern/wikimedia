package main

import (
	"context"
	"fmt"
	"log"

	"github.com/karte-bayern/wikimedia"
)

func main() {
	client, err := wikimedia.New(
		wikimedia.WithUserAgent("wikimedia-basic-example/1.0 (admin@example.org)"),
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
		fmt.Printf("%s · %s · %s\n", image.Attribution, image.LicenseShortName, image.PageURL)
	}
}
