package main

import "testing"

func TestSortProwlarrResultsBySizeDesc(t *testing.T) {
	results := []ProwlarrSearchResult{
		{Title: "small", Size: 1},
		{Title: "big", Size: 3},
		{Title: "mid", Size: 2},
	}

	sorted := sortProwlarrResults(results)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 results, got %d", len(sorted))
	}
	if sorted[0].Title != "big" || sorted[1].Title != "mid" || sorted[2].Title != "small" {
		t.Fatalf("unexpected sort order: %#v", sorted)
	}
}

func TestExtractLanguageFromTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{title: "Movie.2024.1080p.WEB-DL.ENG", want: "English"},
		{title: "Movie.2024.1080p.WEB-DL.SPANISH.LATINO", want: "Spanish"},
		{title: "Movie.2024.1080p.MULTI", want: "Multi Audio"},
		{title: "Movie.2024.1080p.DUAL-AUDIO", want: "Dual Audio"},
		{title: "Movie.2024.1080p.ENGLISH.FRENCH", want: "English, French"},
	}

	for _, tt := range tests {
		got := extractLanguageFromTitle(tt.title)
		if got != tt.want {
			t.Fatalf("extractLanguageFromTitle(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}
