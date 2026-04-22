package compose

import "testing"

func TestNormalizeRepoURL(t *testing.T) {
	cases := []struct {
		in      string
		wantURL string
		wantN   string
		wantErr bool
	}{
		{"iluxa/app1", "git@github.com:iluxa/app1.git", "app1", false},
		{"iluxa/app1.git", "git@github.com:iluxa/app1.git", "app1", false},
		{"git@github.com:iluxa/app1.git", "git@github.com:iluxa/app1.git", "app1", false},
		{"git@gitlab.com:group/sub.git", "git@gitlab.com:group/sub.git", "sub", false},
		{"https://github.com/iluxa/app1", "git@github.com:iluxa/app1.git", "app1", false},
		{"https://github.com/iluxa/app1.git", "git@github.com:iluxa/app1.git", "app1", false},
		{"http://example.com/a/b/", "git@example.com:a/b.git", "b", false},
		{"ssh://git@host/x/y.git", "ssh://git@host/x/y.git", "y", false},
		{"", "", "", true},
		{"no-slash", "", "", true},
	}
	for _, c := range cases {
		url, n, err := NormalizeRepoURL(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: expected error, got url=%s name=%s", c.in, url, n)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error: %v", c.in, err)
			continue
		}
		if url != c.wantURL {
			t.Errorf("%q: url = %q, want %q", c.in, url, c.wantURL)
		}
		if n != c.wantN {
			t.Errorf("%q: name = %q, want %q", c.in, n, c.wantN)
		}
	}
}
