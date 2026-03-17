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

func TestBuildStreamsIncludesQualityInName(t *testing.T) {
	results := []DebridStreamResult{{
		URL:      "https://video.example/stream",
		Filename: "Movie.2024.1080p.WEB-DL.mkv",
		Size:     1024,
		Language: "English",
		Host:     "alldebrid",
	}}

	streams := buildStreams("movie", results)
	if len(streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(streams))
	}
	if streams[0].Name != "⚡Frankie\n1080p" {
		t.Fatalf("unexpected stream name: %q", streams[0].Name)
	}

	expectedDescription := "Movie.2024.1080p.WEB-DL.mkv\n💾 1.0 KiB\n🗣️ English\n🌐 alldebrid"
	if streams[0].Description != expectedDescription {
		t.Fatalf("unexpected stream description: %q", streams[0].Description)
	}
}

func TestSortProwlarrResultsPrefersHigherQuality(t *testing.T) {
	results := []ProwlarrSearchResult{
		{Title: "Movie.2024.1080p.WEB-DL", Size: 1_000},
		{Title: "Movie.2024.2160p.WEB-DL", Size: 900},
		{Title: "Movie.2024.720p.WEB-DL", Size: 2_000},
	}

	sorted := sortProwlarrResults(results)
	if got, want := sorted[0].Title, "Movie.2024.2160p.WEB-DL"; got != want {
		t.Fatalf("unexpected first result %q, want %q", got, want)
	}
	if got, want := sorted[1].Title, "Movie.2024.1080p.WEB-DL"; got != want {
		t.Fatalf("unexpected second result %q, want %q", got, want)
	}
}

func TestSortProwlarrResultsUsesLanguagePreferenceAfterQuality(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)
	setConfig(Config{PrimaryLanguage: "Spanish", SecondaryLanguage: "English"})

	results := []ProwlarrSearchResult{
		{Title: "Movie.2024.1080p.WEB-DL.ENG", Size: 2_000},
		{Title: "Movie.2024.1080p.WEB-DL.SPANISH", Size: 1_000},
		{Title: "Movie.2024.2160p.WEB-DL.ENG", Size: 500},
	}

	sorted := sortProwlarrResults(results)
	if got, want := sorted[0].Title, "Movie.2024.2160p.WEB-DL.ENG"; got != want {
		t.Fatalf("quality should rank first: got %q want %q", got, want)
	}
	if got, want := sorted[1].Title, "Movie.2024.1080p.WEB-DL.SPANISH"; got != want {
		t.Fatalf("language preference should rank second: got %q want %q", got, want)
	}
}

func TestDedupeProwlarrResultsByInfoHash(t *testing.T) {
	results := []ProwlarrSearchResult{
		{Title: "A.1080p", InfoHash: "abc", Size: 100},
		{Title: "A.1080p duplicate", InfoHash: "ABC", Size: 101},
		{Title: "B.2160p", InfoHash: "def", Size: 200},
	}

	deduped := dedupeProwlarrResults(results)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 unique results, got %d", len(deduped))
	}
}
