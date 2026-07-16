package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/karte-bayern/wikimedia"
)

func main() {
	client, err := wikimedia.New(
		wikimedia.WithUserAgent("wikimedia-category-example/1.0 (https://example.org/contact)"),
		wikimedia.WithLanguages("de", "en"),
	)
	if err != nil {
		log.Fatal(err)
	}
	result, err := client.Fetch(
		context.Background(), "Q82425",
		wikimedia.WithCommonsCategories(true),
		wikimedia.WithMediaLimit(12),
		wikimedia.WithCategoryPageLimit(2),
	)
	if err != nil {
		log.Fatal(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result.Media); err != nil {
		log.Fatal(err)
	}
}
