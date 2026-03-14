package main

import "testing"

func TestFilterResultsByTitleSimilarity(t *testing.T) {
	results := []ProwlarrSearchResult{
		{Title: "Dune Part Two 2024 2160p WEB-DL"},
		{Title: "Mad Max Fury Road 2015 1080p BluRay"},
		{Title: "Dune Prophecy S01E01 1080p WEBRip"},
	}

	filtered := filterResultsByTitleSimilarity(results, "Dune: Part Two")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered result, got %d", len(filtered))
	}
	if filtered[0].Title != "Dune Part Two 2024 2160p WEB-DL" {
		t.Fatalf("unexpected filtered result title: %q", filtered[0].Title)
	}
}

func TestTitleSimilarityCompactTokens(t *testing.T) {
	expected := tokenizeTitle("Spider-Man")
	score := titleSimilarity(expected, "Spiderman No Way Home 2021 2160p")
	if score < 0.6 {
		t.Fatalf("expected compact title match score >= 0.6, got %.2f", score)
	}
}

func TestFilterResultsByTitleSimilarityEmptyExpected(t *testing.T) {
	results := []ProwlarrSearchResult{{Title: "Anything 1080p"}}
	filtered := filterResultsByTitleSimilarity(results, "")
	if len(filtered) != 1 {
		t.Fatalf("expected unfiltered results when expected title is empty, got %d", len(filtered))
	}
}
