package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildAdvisoryQuery(t *testing.T) {
	query, variables := buildAdvisoryQuery([]string{"actions/checkout", "actions/setup-go"})

	require.Contains(t, query, "a0: securityVulnerabilities(ecosystem: ACTIONS, package: $p0")
	require.Contains(t, query, "a1: securityVulnerabilities(ecosystem: ACTIONS, package: $p1")
	require.Equal(t, "actions/checkout", variables["p0"])
	require.Equal(t, "actions/setup-go", variables["p1"])
}
