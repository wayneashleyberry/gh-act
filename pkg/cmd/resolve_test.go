package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wayneashleyberry/gh-act/pkg/api"
)

func TestMatchPinnedTagPrefersHighestSemver(t *testing.T) {
	const sha = "0123456789012345678901234567890123456789"

	tests := []struct {
		name     string
		tags     []api.Tag
		expected string
	}{
		{
			name: "more specific version on same commit",
			tags: []api.Tag{
				{Name: "v7", Commit: api.Commit{Sha: sha}},
				{Name: "v7.0.1", Commit: api.Commit{Sha: sha}},
			},
			expected: "v7.0.1",
		},
		{
			name: "double-digit major is not beaten by longer string",
			tags: []api.Tag{
				{Name: "v9.9", Commit: api.Commit{Sha: sha}},
				{Name: "v10", Commit: api.Commit{Sha: sha}},
			},
			expected: "v10",
		},
		{
			name: "falls back to longest name for non-semver tags",
			tags: []api.Tag{
				{Name: "stable", Commit: api.Commit{Sha: sha}},
				{Name: "stable-lts", Commit: api.Commit{Sha: sha}},
			},
			expected: "stable-lts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, err := matchPinnedTag(sha, tt.tags, "owner/repo")
			require.NoError(t, err)
			require.Equal(t, tt.expected, matched.GetName())
		})
	}
}

func TestMatchPinnedTagNoMatch(t *testing.T) {
	tags := []api.Tag{{Name: "v1.0.0", Commit: api.Commit{Sha: "aaa"}}}

	_, err := matchPinnedTag("bbb", tags, "owner/repo")
	require.Error(t, err)
}

func TestValidateReference(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		wantErr bool
	}{
		{name: "valid", action: "actions/checkout", wantErr: false},
		{name: "valid with subpath", action: "aws-actions/configure/setup", wantErr: false},
		{name: "valid with dots", action: "owner/repo.js", wantErr: false},
		{name: "path traversal in repo", action: "owner/..", wantErr: true},
		{name: "path traversal in subpath", action: "owner/repo/..", wantErr: true},
		{name: "slash injection", action: "owner/repo name", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := splitReference(tt.action)

			err := validateReference(parts)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// splitReference builds a ParsedAction from an owner/repo(/subpath) string for
// validation testing.
func splitReference(ref string) ParsedAction {
	parsed := ParsedAction{}

	segments := strings.Split(ref, "/")
	if len(segments) > 0 {
		parsed.Owner = segments[0]
	}

	if len(segments) > 1 {
		parsed.Repo = segments[1]
	}

	if len(segments) > 2 {
		parsed.Subpath = strings.Join(segments[2:], "/")
	}

	return parsed
}
