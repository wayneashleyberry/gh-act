package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/wayneashleyberry/gh-act/pkg/api"
)

// severityRank orders GHSA severities from least to most severe. Unknown
// severities rank below "low".
var severityRank = map[string]int{
	"LOW":      1,
	"MODERATE": 2,
	"HIGH":     3,
	"CRITICAL": 4,
}

// AuditActions reports every used action whose currently referenced version
// is affected by a known security advisory, sourced from the GitHub Security
// Advisory Database (GHSA) for the "actions" ecosystem. minSeverity filters
// out advisories below that severity (low, moderate, high or critical).
// Returns whether any advisory was found.
func AuditActions(ctx context.Context, opts CollectOptions, minSeverity string) (bool, error) {
	client, err := api.NewClient()
	if err != nil {
		return false, fmt.Errorf("create github client: %w", err)
	}

	_, refs, err := collectActionRefs(opts)
	if err != nil {
		return false, err
	}

	actions, err := resolveActions(ctx, refs, client)
	if err != nil {
		return false, err
	}

	packages := advisoryPackages(actions)

	vulnsByPackage, err := client.FetchVulnerabilities(ctx, packages)
	if err != nil {
		return false, fmt.Errorf("fetch security advisories: %w", err)
	}

	found := false

	for _, action := range actions {
		if action.CurrentVersionTag == nil {
			continue
		}

		for _, vuln := range vulnsByPackage[action.Owner+"/"+action.Repo] {
			if !severityAtLeast(vuln.Severity, minSeverity) {
				continue
			}

			affected, err := versionInRange(action.CurrentVersionTag, vuln.VulnerableVersionRange)
			if err != nil {
				slog.Debug(
					"could not check vulnerable version range",
					slog.String("action", action.ActionReference()),
					slog.String("range", vuln.VulnerableVersionRange),
					slog.String("error.message", err.Error()),
				)

				continue
			}

			if !affected {
				continue
			}

			found = true

			printAdvisory(action, vuln)
		}
	}

	return found, nil
}

// advisoryPackages returns the deduplicated "owner/repo" packages (subpaths
// stripped, since advisories are published against the repository, not an
// individual composite action within it) referenced by actions.
func advisoryPackages(actions []ParsedAction) []string {
	seen := make(map[string]bool)

	var packages []string

	for _, action := range actions {
		pkg := action.Owner + "/" + action.Repo

		if !seen[pkg] {
			seen[pkg] = true

			packages = append(packages, pkg)
		}
	}

	return packages
}

// severityAtLeast reports whether got meets or exceeds the min severity
// threshold. Both are matched case-insensitively; an unrecognised min value
// never filters anything out.
func severityAtLeast(got, minSeverity string) bool {
	minRank, ok := severityRank[strings.ToUpper(minSeverity)]
	if !ok {
		return true
	}

	return severityRank[strings.ToUpper(got)] >= minRank
}

// versionInRange reports whether tag's version falls within a GHSA
// vulnerable version range expression (e.g. "< 1.2.3", ">= 1.0.0, < 1.0.5").
func versionInRange(tag *api.Tag, rangeExpr string) (bool, error) {
	version, err := semver.NewVersion(tag.GetName())
	if err != nil {
		return false, fmt.Errorf("parse tag version: %w", err)
	}

	constraint, err := semver.NewConstraint(rangeExpr)
	if err != nil {
		return false, fmt.Errorf("parse vulnerable version range %q: %w", rangeExpr, err)
	}

	return constraint.Check(version), nil
}

// printAdvisory prints a single matched advisory as a short indented block:
// a located header line followed by the severity/CVE/summary and a link to
// the advisory with the patched version, if known.
func printAdvisory(action ParsedAction, vuln api.Vulnerability) {
	location := fmt.Sprintf("%s:%d:%d", action.FilePath, action.Node.Line, action.Node.Column)

	fmt.Printf("%s: %s@%s\n", location, action.ActionReference(), action.CurrentVersionTag.GetName())

	id := strings.Join(vuln.CVEs(), ", ")
	if id == "" {
		id = "no CVE assigned"
	}

	fmt.Printf("  %s  %s: %s\n", vuln.Severity, id, vuln.Summary)

	fix := "no fix published yet"
	if vuln.FirstPatchedVersion != "" {
		fix = "fixed in " + vuln.FirstPatchedVersion
	}

	fmt.Printf("  %s — %s\n", fix, vuln.Permalink)
}
