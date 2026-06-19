package model

import (
	"path/filepath"
	"testing"
)

func TestProjectsUnder(t *testing.T) {
	root := filepath.FromSlash("/home/u/code")
	projects := []Project{
		{ID: "root", Path: root},
		{ID: "child", Path: filepath.Join(root, "alpha")},
		{ID: "deep", Path: filepath.Join(root, "x", "y", "z")},
		{ID: "sibling", Path: filepath.FromSlash("/home/u/other")},
		{ID: "parent", Path: filepath.FromSlash("/home/u")},
		{ID: "prefixonly", Path: filepath.FromSlash("/home/u/code-extra")},
	}

	idsOf := func(ps []Project) []string {
		ids := make([]string, len(ps))
		for i, p := range ps {
			ids[i] = p.ID
		}
		return ids
	}

	tests := []struct {
		name string
		dir  string
		want []string
	}{
		{"recursive at and under folder", root, []string{"root", "child", "deep"}},
		{"empty dir returns all", "", []string{"root", "child", "deep", "sibling", "parent", "prefixonly"}},
		{"non-matching folder returns empty", filepath.FromSlash("/nope"), []string{}},
		{"prefix-only is not under", filepath.Join(root, "x"), []string{"deep"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := idsOf(ProjectsUnder(projects, tt.dir))
			if len(got) != len(tt.want) {
				t.Fatalf("ProjectsUnder(%q) = %v, want %v", tt.dir, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ProjectsUnder(%q) = %v, want %v", tt.dir, got, tt.want)
				}
			}
		})
	}
}
