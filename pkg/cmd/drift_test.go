package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// action builds a raw Action reference for drift tests.
func action(filePath, value, lineComment string) Action {
	return Action{
		FilePath: filePath,
		Node:     yaml.Node{Value: value, LineComment: lineComment},
	}
}

func TestGroupDriftEntriesNoDrift(t *testing.T) {
	refs := []Action{
		action("a.yml", "actions/setup-go@v5", ""),
		action("b.yml", "actions/setup-go@v5", ""),
	}

	groups := groupDriftEntries(refs)
	require.Len(t, groups, 1)
	require.False(t, groups[0].hasDrift())
}

func TestGroupDriftEntriesVersionDrift(t *testing.T) {
	refs := []Action{
		action("a.yml", "actions/checkout@v4", ""),
		action("b.yml", "actions/checkout@v3", ""),
	}

	groups := groupDriftEntries(refs)
	require.Len(t, groups, 1)
	require.True(t, groups[0].hasDrift())
}

func TestGroupDriftEntriesStyleDrift(t *testing.T) {
	sha := "0123456789012345678901234567890123456789"
	refs := []Action{
		action("a.yml", "actions/checkout@v4", ""),
		action("b.yml", "actions/checkout@"+sha, ""),
	}

	groups := groupDriftEntries(refs)
	require.Len(t, groups, 1)
	require.True(t, groups[0].hasDrift())
}

func TestGroupDriftEntriesDifferentRepos(t *testing.T) {
	refs := []Action{
		action("a.yml", "actions/checkout@v4", ""),
		action("b.yml", "actions/setup-go@v5", ""),
	}

	groups := groupDriftEntries(refs)
	require.Len(t, groups, 2)
	require.False(t, groups[0].hasDrift())
	require.False(t, groups[1].hasDrift())
}

func TestGroupDriftEntriesNopinExcluded(t *testing.T) {
	refs := []Action{
		action("a.yml", "actions/checkout@v4", ""),
		action("b.yml", "actions/checkout@main", "# nopin"),
	}

	groups := groupDriftEntries(refs)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Entries, 1)
	require.False(t, groups[0].hasDrift())
}

func TestGroupDriftEntriesCaseInsensitiveGrouping(t *testing.T) {
	refs := []Action{
		action("a.yml", "Actions/Checkout@v4", ""),
		action("b.yml", "actions/checkout@v3", ""),
	}

	groups := groupDriftEntries(refs)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Entries, 2)
	require.True(t, groups[0].hasDrift())
}

func TestGroupDriftEntriesSkipsLocalAndDocker(t *testing.T) {
	refs := []Action{
		action("a.yml", "./local-action", ""),
		action("a.yml", "docker://alpine:3", ""),
	}

	groups := groupDriftEntries(refs)
	require.Empty(t, groups)
}
