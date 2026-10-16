package ghclient

import (
	"testing"

	"github.com/google/go-github/v92/github"
)

func TestMergeMethod(t *testing.T) {
	tests := []struct {
		name                  string
		merge, squash, rebase bool
		want                  string
	}{
		{"all allowed keeps the merge commit", true, true, true, "merge"},
		{"squash only", false, true, false, "squash"},
		{"rebase only", false, false, true, "rebase"},
		{"squash preferred over rebase", false, true, true, "squash"},
	}
	for _, tt := range tests {
		r := &github.Repository{
			AllowMergeCommit: github.Ptr(tt.merge),
			AllowSquashMerge: github.Ptr(tt.squash),
			AllowRebaseMerge: github.Ptr(tt.rebase),
		}
		if got := mergeMethod(r); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
