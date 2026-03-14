package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestFlattenMagnetFiles(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    []MagnetFile
	}{
		{
			name: "single file at root",
			payload: `[
				{"n":"some.file.avi","s":45466546,"l":"https://alldebrid.com/f/4564654"}
			]`,
			want: []MagnetFile{
				{Name: "some.file.avi", Size: 45466546, Link: "https://alldebrid.com/f/4564654"},
			},
		},
		{
			name: "single file in deeply nested subfolder",
			payload: `[
				{"n":"subfolderName","e":[{"n":"deepSubfolder","e":[{"n":"some.file.avi","s":45466546,"l":"https://alldebrid.com/f/4564654"}]}]}
			]`,
			want: []MagnetFile{
				{Name: "some.file.avi", Size: 45466546, Link: "https://alldebrid.com/f/4564654"},
			},
		},
		{
			name: "multiple files in folders and root",
			payload: `[
				{
					"n": "subfolderName",
					"e": [
						{
							"n": "deepSubfolder",
							"e": [
								{"n": "some.file.txt", "s": 456546, "l": "https://alldebrid.com/f/45654"}
							]
						},
						{
							"n": "otherSubfolder",
							"e": [
								{"n": "other.file.txt", "s": 12211, "l": "https://alldebrid.com/f/111111"}
							]
						}
					]
				},
				{"n": "file.at.root.avi", "s": 1000000000, "l": "https://alldebrid.com/f/5555555"}
			]`,
			want: []MagnetFile{
				{Name: "some.file.txt", Size: 456546, Link: "https://alldebrid.com/f/45654"},
				{Name: "other.file.txt", Size: 12211, Link: "https://alldebrid.com/f/111111"},
				{Name: "file.at.root.avi", Size: 1000000000, Link: "https://alldebrid.com/f/5555555"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := flattenMagnetFiles(json.RawMessage(tt.payload))
			if err != nil {
				t.Fatalf("flattenMagnetFiles() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("flattenMagnetFiles() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
