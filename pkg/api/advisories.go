package api

import (
	"context"
	"fmt"
	"strings"
)

// advisoryPageSize is the number of vulnerabilities requested per package in a
// single securityVulnerabilities query.
const advisoryPageSize = 100

// actionsEcosystem is the GHSA SecurityAdvisoryEcosystem enum value for
// GitHub Actions.
const actionsEcosystem = "ACTIONS"

// Identifier is an advisory identifier, such as a CVE or GHSA id.
type Identifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// Vulnerability describes a single security advisory affecting a version
// range of an action.
type Vulnerability struct {
	Severity               string       `json:"severity"`
	Summary                string       `json:"summary"`
	Permalink              string       `json:"permalink"`
	Identifiers            []Identifier `json:"identifiers"`
	VulnerableVersionRange string       `json:"vulnerableVersionRange"`
	FirstPatchedVersion    string       `json:"firstPatchedVersion"`
}

// CVEs returns every CVE identifier attached to the advisory.
func (v Vulnerability) CVEs() []string {
	var cves []string

	for _, id := range v.Identifiers {
		if id.Type == "CVE" {
			cves = append(cves, id.Value)
		}
	}

	return cves
}

// vulnerabilityNode mirrors the GraphQL SecurityVulnerability shape. A null
// firstPatchedVersion is a no-op for encoding/json against a non-pointer
// struct field, leaving it at its zero value, so no nil check is needed.
type vulnerabilityNode struct {
	Severity               string `json:"severity"`
	VulnerableVersionRange string `json:"vulnerableVersionRange"`
	FirstPatchedVersion    struct {
		Identifier string `json:"identifier"`
	} `json:"firstPatchedVersion"`
	Advisory struct {
		Summary     string       `json:"summary"`
		Permalink   string       `json:"permalink"`
		Identifiers []Identifier `json:"identifiers"`
	} `json:"advisory"`
}

// FetchVulnerabilities returns every known security advisory for each
// owner/repo package in the GitHub Actions advisory ecosystem, keyed by
// "owner/repo". Packages with no known advisories are omitted from the
// result. Results are cached and de-duplicated for the lifetime of the
// Client.
func (c *Client) FetchVulnerabilities(ctx context.Context, packages []string) (map[string][]Vulnerability, error) {
	result := make(map[string][]Vulnerability)

	toFetch := make([]string, 0, len(packages))

	c.mu.Lock()
	for _, pkg := range packages {
		if vulns, ok := c.advisoryCache[pkg]; ok {
			if len(vulns) > 0 {
				result[pkg] = vulns
			}

			continue
		}

		toFetch = append(toFetch, pkg)
	}
	c.mu.Unlock()

	if len(toFetch) > 0 {
		fetched, err := c.fetchVulnerabilityBatch(ctx, toFetch)
		if err != nil {
			return nil, err
		}

		c.mu.Lock()
		for _, pkg := range toFetch {
			c.advisoryCache[pkg] = fetched[pkg]
		}
		c.mu.Unlock()

		for pkg, vulns := range fetched {
			if len(vulns) > 0 {
				result[pkg] = vulns
			}
		}
	}

	return result, nil
}

// fetchVulnerabilityBatch queries advisories for a single batch of packages in
// one GraphQL request, using one aliased securityVulnerabilities field per
// package.
func (c *Client) fetchVulnerabilityBatch(ctx context.Context, packages []string) (map[string][]Vulnerability, error) {
	query, variables := buildAdvisoryQuery(packages)

	response := make(map[string]struct {
		Nodes []vulnerabilityNode `json:"nodes"`
	})

	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return nil, fmt.Errorf("fetch security advisories: %w", err)
	}

	result := make(map[string][]Vulnerability, len(packages))

	for i, pkg := range packages {
		alias := fmt.Sprintf("a%d", i)

		var vulns []Vulnerability

		for _, node := range response[alias].Nodes {
			vulns = append(vulns, Vulnerability{
				Severity:               node.Severity,
				Summary:                node.Advisory.Summary,
				Permalink:              node.Advisory.Permalink,
				Identifiers:            node.Advisory.Identifiers,
				VulnerableVersionRange: node.VulnerableVersionRange,
				FirstPatchedVersion:    node.FirstPatchedVersion.Identifier,
			})
		}

		result[pkg] = vulns
	}

	return result, nil
}

// buildAdvisoryQuery builds a GraphQL query that fetches advisories for every
// package in a single request, aliasing each securityVulnerabilities field as
// a0, a1, ... in input order.
func buildAdvisoryQuery(packages []string) (string, map[string]any) {
	variables := make(map[string]any, len(packages))

	var params, fields strings.Builder

	for i, pkg := range packages {
		name := fmt.Sprintf("p%d", i)
		alias := fmt.Sprintf("a%d", i)

		variables[name] = pkg

		fmt.Fprintf(&params, "$%s: String!, ", name)
		fmt.Fprintf(
			&fields,
			`%s: securityVulnerabilities(ecosystem: %s, package: $%s, first: %d) {
				nodes {
					severity
					vulnerableVersionRange
					firstPatchedVersion { identifier }
					advisory {
						summary
						permalink
						identifiers { type value }
					}
				}
			}
			`,
			alias, actionsEcosystem, name, advisoryPageSize,
		)
	}

	query := fmt.Sprintf("query(%s) {\n%s}", params.String(), fields.String())

	return query, variables
}
