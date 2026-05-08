package deploy

import (
	"reflect"
	"testing"
)

func TestFilterAutoVolumePaths(t *testing.T) {
	cases := []struct {
		name     string
		declared []string
		existing map[string]bool
		want     []string
	}{
		{
			name:     "user mapped one of two — only unmapped survives",
			declared: []string{"/data", "/cache"},
			existing: map[string]bool{"/cache": true},
			want:     []string{"/data"},
		},
		{
			name:     "user mapped all — empty result",
			declared: []string{"/data"},
			existing: map[string]bool{"/data": true},
			want:     []string{},
		},
		{
			name:     "no existing mappings — all pass through",
			declared: []string{"/var/lib/postgresql/data", "/data"},
			existing: nil,
			want:     []string{"/data", "/var/lib/postgresql/data"}, // sorted
		},
		{
			name:     "empty declared — empty result",
			declared: nil,
			existing: nil,
			want:     []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterAutoVolumePaths(tc.declared, tc.existing)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
