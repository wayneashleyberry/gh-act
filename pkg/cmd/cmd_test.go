package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wayneashleyberry/gh-act/pkg/api"
)

// testFetchAllTags is a mock for api.FetchAllTags that returns a single tag matching the version in the action string.
func testFetchAllTags(_ context.Context, owner, repo string) ([]api.Tag, error) {
	// Return a tag for the most common test cases
	switch repo {
	case "checkout":
		return []api.Tag{{
			Name:   "v3.2.1",
			Commit: api.Commit{Sha: "dummysha1"},
		}}, nil
	case "setup-go":
		return []api.Tag{{
			Name:   "v4",
			Commit: api.Commit{Sha: "dummysha2"},
		}}, nil
	case "cache":
		return []api.Tag{{
			Name:   "v3",
			Commit: api.Commit{Sha: "dummysha3"},
		}}, nil
	case "upload-artifact":
		return []api.Tag{{
			Name:   "v4.3.1",
			Commit: api.Commit{Sha: "dummysha4"},
		}}, nil
	default:
		// Return a generic tag for any other repo
		return []api.Tag{{
			Name:   "v1.0.0",
			Commit: api.Commit{Sha: "dummysha"},
		}}, nil
	}
}

func patchFetchAllTags(t *testing.T) func() {
	orig := api.FetchAllTags
	api.FetchAllTags = testFetchAllTags
	return func() { api.FetchAllTags = orig }
}

func TestParseActionsInString_BasicJobStep(t *testing.T) {
	defer patchFetchAllTags(t)()
	yaml := `
jobs:
  build:
    steps:
      - uses: actions/checkout@v3.2.1
      - uses: actions/setup-go@v4
`
	ctx := context.Background()
	parsed, err := parseActionsInString(ctx, yaml, "test.yml")
	require.NoError(t, err)
	require.Len(t, parsed, 2)

	require.Equal(t, "actions", parsed[0].Owner)
	require.Equal(t, "checkout", parsed[0].Repo)
	require.Equal(t, "v3.2.1", parsed[0].RawVersionString)

	require.Equal(t, "actions", parsed[1].Owner)
	require.Equal(t, "setup-go", parsed[1].Repo)
	require.Equal(t, "v4", parsed[1].RawVersionString)
}

func TestParseActionsInString_IgnoresLocalAndNested(t *testing.T) {
	defer patchFetchAllTags(t)()
	yaml := `
jobs:
  build:
    steps:
      - uses: ./local-action
      - uses: foo/bar/baz@v1
      - uses: actions/cache@v3
`
	ctx := context.Background()
	parsed, err := parseActionsInString(ctx, yaml, "test.yml")
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "actions", parsed[0].Owner)
	require.Equal(t, "cache", parsed[0].Repo)
	require.Equal(t, "v3", parsed[0].RawVersionString)
}

func TestParseActionsInString_RunsSteps(t *testing.T) {
	defer patchFetchAllTags(t)()
	yaml := `
runs:
  steps:
    - uses: actions/upload-artifact@v4.3.1
`
	ctx := context.Background()
	parsed, err := parseActionsInString(ctx, yaml, "test.yml")
	require.NoError(t, err)
	require.Len(t, parsed, 1)
	require.Equal(t, "actions", parsed[0].Owner)
	require.Equal(t, "upload-artifact", parsed[0].Repo)
	require.Equal(t, "v4.3.1", parsed[0].RawVersionString)
}
