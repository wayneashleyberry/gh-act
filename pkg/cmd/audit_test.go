package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wayneashleyberry/gh-act/pkg/api"
)

func TestSeverityAtLeast(t *testing.T) {
	tests := []struct {
		name string
		got  string
		min  string
		want bool
	}{
		{name: "equal", got: "HIGH", min: "high", want: true},
		{name: "above", got: "CRITICAL", min: "high", want: true},
		{name: "below", got: "LOW", min: "high", want: false},
		{name: "case insensitive", got: "moderate", min: "MODERATE", want: true},
		{name: "unknown min never filters", got: "LOW", min: "nonsense", want: true},
		{name: "unknown got ranks lowest", got: "nonsense", min: "low", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, severityAtLeast(tt.got, tt.min))
		})
	}
}

func TestVersionInRange(t *testing.T) {
	tests := []struct {
		name    string
		tag     string
		rng     string
		want    bool
		wantErr bool
	}{
		{name: "below exclusive upper bound", tag: "v1.0.0", rng: "< 1.0.5", want: true},
		{name: "at exclusive upper bound", tag: "v1.0.5", rng: "< 1.0.5", want: false},
		{name: "within AND range", tag: "v1.0.3", rng: ">= 1.0.0, < 1.0.5", want: true},
		{name: "outside AND range", tag: "v2.0.0", rng: ">= 1.0.0, < 1.0.5", want: false},
		{name: "unparsable range", tag: "v1.0.0", rng: "not a range", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := &api.Tag{Name: tt.tag}

			got, err := versionInRange(tag, tt.rng)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAdvisoryPackages(t *testing.T) {
	actions := []ParsedAction{
		{Owner: "actions", Repo: "checkout", Subpath: ""},
		{Owner: "actions", Repo: "checkout", Subpath: ""},
		{Owner: "actions", Repo: "setup-go"},
		{Owner: "my-org", Repo: "monorepo", Subpath: "composite/action"},
	}

	got := advisoryPackages(actions)

	require.Equal(t, []string{"actions/checkout", "actions/setup-go", "my-org/monorepo"}, got)
}
