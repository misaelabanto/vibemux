package gitops

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		in      string
		wantURL string
		wantDir string
		wantErr bool
	}{
		{"misaelabanto/vibemux", "git@github.com:misaelabanto/vibemux.git", "vibemux", false},
		{"foo-bar/baz_qux.x", "git@github.com:foo-bar/baz_qux.x.git", "baz_qux.x", false},
		{"git@github.com:misaelabanto/vibemux.git", "git@github.com:misaelabanto/vibemux.git", "vibemux", false},
		{"git@gitlab.example.com:group/project.git", "git@gitlab.example.com:group/project.git", "project", false},
		{"https://github.com/owner/repo.git", "git@github.com:owner/repo.git", "repo", false},
		{"https://github.com/owner/repo", "git@github.com:owner/repo.git", "repo", false},
		{"https://github.com/owner/repo/", "git@github.com:owner/repo.git", "repo", false},
		{"ssh://git@github.com/owner/repo.git", "ssh://git@github.com/owner/repo.git", "repo", false},
		{"https://gitlab.com/group/project.git", "https://gitlab.com/group/project.git", "project", false},
		{"  misaelabanto/vibemux  ", "git@github.com:misaelabanto/vibemux.git", "vibemux", false},
		{"", "", "", true},
	}
	for _, c := range cases {
		gotURL, gotDir, err := NormalizeURL(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("NormalizeURL(%q) err=%v, wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if c.wantErr {
			continue
		}
		if gotURL != c.wantURL {
			t.Errorf("NormalizeURL(%q) url=%q, want %q", c.in, gotURL, c.wantURL)
		}
		if gotDir != c.wantDir {
			t.Errorf("NormalizeURL(%q) dir=%q, want %q", c.in, gotDir, c.wantDir)
		}
	}
}
