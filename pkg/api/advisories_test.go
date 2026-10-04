package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeGraphQLClient captures the query/variables it was called with and
// returns a canned response, to test batch-building and response parsing
// without a live HTTP call.
type fakeGraphQLClient struct {
	query     string
	variables map[string]any
	respond   func(response any)
	err       error
}

func (f *fakeGraphQLClient) DoWithContext(_ context.Context, query string, variables map[string]any, response any) error {
	f.query = query
	f.variables = variables

	if f.err != nil {
		return f.err
	}

	if f.respond != nil {
		f.respond(response)
	}

	return nil
}

func TestBuildAdvisoryQuery(t *testing.T) {
	query, variables := buildAdvisoryQuery([]string{"actions/checkout", "actions/setup-go"})

	require.Contains(t, query, "a0: securityVulnerabilities(ecosystem: ACTIONS, package: $p0")
	require.Contains(t, query, "a1: securityVulnerabilities(ecosystem: ACTIONS, package: $p1")
	require.Equal(t, "actions/checkout", variables["p0"])
	require.Equal(t, "actions/setup-go", variables["p1"])
}

func TestFetchVulnerabilitiesParsesResponseAndCaches(t *testing.T) {
	ctx := context.Background()

	fake := &fakeGraphQLClient{
		respond: func(response any) {
			out, ok := response.(*map[string]struct {
				Nodes []vulnerabilityNode `json:"nodes"`
			})
			require.True(t, ok)

			(*out)["a0"] = struct {
				Nodes []vulnerabilityNode `json:"nodes"`
			}{
				Nodes: []vulnerabilityNode{{
					Severity:               "HIGH",
					VulnerableVersionRange: "< 1.0.1",
					Advisory: struct {
						Summary     string       `json:"summary"`
						Permalink   string       `json:"permalink"`
						Identifiers []Identifier `json:"identifiers"`
					}{
						Summary:     "example advisory",
						Permalink:   "https://github.com/advisories/GHSA-xxxx",
						Identifiers: []Identifier{{Type: "CVE", Value: "CVE-2024-1234"}},
					},
				}},
			}
		},
	}

	client := &Client{
		graphql:       fake,
		tagCache:      make(map[string][]Tag),
		repoCache:     make(map[string]*Repository),
		advisoryCache: make(map[string][]Vulnerability),
	}

	result, err := client.FetchVulnerabilities(ctx, []string{"actions/checkout"})
	require.NoError(t, err)
	require.Len(t, result["actions/checkout"], 1)

	vuln := result["actions/checkout"][0]
	require.Equal(t, "HIGH", vuln.Severity)
	require.Equal(t, "< 1.0.1", vuln.VulnerableVersionRange)
	require.Equal(t, []string{"CVE-2024-1234"}, vuln.CVEs())

	// Second call for the same package must be served from the cache, not
	// trigger another GraphQL request.
	fake.query = ""

	result, err = client.FetchVulnerabilities(ctx, []string{"actions/checkout"})
	require.NoError(t, err)
	require.Len(t, result["actions/checkout"], 1)
	require.Empty(t, fake.query)
}

func TestFetchVulnerabilitiesOmitsPackagesWithNoAdvisories(t *testing.T) {
	ctx := context.Background()

	fake := &fakeGraphQLClient{}

	client := &Client{
		graphql:       fake,
		tagCache:      make(map[string][]Tag),
		repoCache:     make(map[string]*Repository),
		advisoryCache: make(map[string][]Vulnerability),
	}

	result, err := client.FetchVulnerabilities(ctx, []string{"actions/checkout"})
	require.NoError(t, err)
	require.Empty(t, result)
}
