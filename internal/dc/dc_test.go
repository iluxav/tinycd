package dc

import (
	"reflect"
	"testing"
)

func TestParseServiceImages(t *testing.T) {
	cases := []struct {
		name    string
		project string
		json    string
		want    map[string]string
	}{
		{
			name:    "image only",
			project: "tcd",
			json:    `{"services": {"db": {"image": "postgres:16"}}}`,
			want:    map[string]string{"db": "postgres:16"},
		},
		{
			name:    "build only — synthesize <project>-<service>:latest",
			project: "tcd",
			json:    `{"services": {"action-notes": {"build": {"context": "."}}}}`,
			want:    map[string]string{"action-notes": "tcd-action-notes:latest"},
		},
		{
			name:    "build + image — explicit image wins",
			project: "tcd",
			json:    `{"services": {"web": {"image": "myorg/web:1", "build": {"context": "."}}}}`,
			want:    map[string]string{"web": "myorg/web:1"},
		},
		{
			name:    "neither image nor build — service skipped",
			project: "tcd",
			json:    `{"services": {"ghost": {}}}`,
			want:    map[string]string{},
		},
		{
			name:    "mixed",
			project: "tcd",
			json:    `{"services": {"db": {"image": "postgres:16"}, "web": {"build": "."}}}`,
			want:    map[string]string{"db": "postgres:16", "web": "tcd-web:latest"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseServiceImages([]byte(tc.json), tc.project)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
