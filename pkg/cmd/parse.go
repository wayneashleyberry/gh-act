package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// githubDir holds workflows and other configuration. Every YAML file beneath it
// is scanned for action references.
const githubDir = ".github"

// skippedWalkDirs are directories excluded from the repository-wide search for
// composite action definitions, to avoid scanning version-control internals and
// vendored third-party code.
var skippedWalkDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

// Action is a raw `uses:` reference discovered in a YAML file, before any
// network resolution has happened.
type Action struct {
	FilePath string
	Node     yaml.Node
}

// findWorkflowFiles returns every YAML file that may contain action references:
//
//   - all *.yml / *.yaml files anywhere under .github (workflows, including
//     nested ones, composite actions, and any other configuration), and
//   - every composite action definition (action.yml / action.yaml) elsewhere in
//     the repository, for example at the root or within a monorepo subtree.
//
// The first set preserves the original scanning scope in full; the second
// extends it to composite actions outside .github. The .git, node_modules and
// vendor directories are skipped during the repository-wide search, and a
// missing .github directory is not an error.
func findWorkflowFiles() ([]string, error) {
	seen := make(map[string]bool)

	var files []string

	add := func(path string) {
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}

	// Every YAML file under .github, recursively (the historical scope).
	if err := filepath.WalkDir(githubDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // tolerate unreadable entries and a missing .github
		}

		if !entry.IsDir() && isYAMLFile(entry.Name()) {
			add(path)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("scan %q: %w", githubDir, err)
	}

	// Composite action definitions anywhere else in the repository.
	if err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // tolerate unreadable entries
		}

		if entry.IsDir() {
			if skippedWalkDirs[entry.Name()] {
				return filepath.SkipDir
			}

			return nil
		}

		if name := entry.Name(); name == "action.yml" || name == "action.yaml" {
			add(path)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("scan for composite actions: %w", err)
	}

	return files, nil
}

func isYAMLFile(name string) bool {
	ext := filepath.Ext(name)

	return ext == ".yml" || ext == ".yaml"
}

// collectActionRefs discovers every workflow/composite file and returns the
// file list (in scan order) alongside the flat list of action references found
// across them.
func collectActionRefs() ([]string, []Action, error) {
	files, err := findWorkflowFiles()
	if err != nil {
		return nil, nil, fmt.Errorf("find workflow files: %w", err)
	}

	var refs []Action

	for _, filePath := range files {
		found, err := findActionRefsInFile(filePath)
		if err != nil {
			return nil, nil, err
		}

		refs = append(refs, found...)
	}

	return files, refs, nil
}

// findActionRefsInFile parses a single YAML file and returns the external
// action references it contains.
func findActionRefsInFile(filePath string) ([]Action, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", filePath, err)
	}

	return parseActionRefs(data, filePath)
}

// parseActionRefs extracts external `uses:` references from workflow and
// composite-action YAML. It understands job steps (jobs.*.steps[].uses),
// reusable workflow calls (jobs.*.uses) and composite action steps
// (runs.steps[].uses). Local (./, docker://) references are skipped.
func parseActionRefs(data []byte, filePath string) ([]Action, error) {
	var doc yaml.Node

	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal YAML in %q: %w", filePath, err)
	}

	if len(doc.Content) == 0 {
		return nil, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}

	var actions []Action

	if jobs := mappingValue(root, "jobs"); jobs != nil && jobs.Kind == yaml.MappingNode {
		for i := 1; i < len(jobs.Content); i += 2 {
			job := jobs.Content[i]
			if job.Kind != yaml.MappingNode {
				continue
			}

			// Reusable workflow call: jobs.<id>.uses.
			if uses := mappingValue(job, "uses"); uses != nil {
				actions = appendUses(actions, filePath, uses)
			}

			if steps := mappingValue(job, "steps"); steps != nil {
				actions = appendStepActions(actions, filePath, steps)
			}
		}
	}

	// Composite action steps: runs.steps[].uses.
	if runs := mappingValue(root, "runs"); runs != nil && runs.Kind == yaml.MappingNode {
		if steps := mappingValue(runs, "steps"); steps != nil {
			actions = appendStepActions(actions, filePath, steps)
		}
	}

	return actions, nil
}

// appendStepActions appends the `uses:` reference of every step in a steps
// sequence node.
func appendStepActions(actions []Action, filePath string, steps *yaml.Node) []Action {
	if steps.Kind != yaml.SequenceNode {
		return actions
	}

	for _, step := range steps.Content {
		if step.Kind != yaml.MappingNode {
			continue
		}

		if uses := mappingValue(step, "uses"); uses != nil {
			actions = appendUses(actions, filePath, uses)
		}
	}

	return actions
}

// appendUses appends a single uses node. Every reference is captured here,
// including local (./…) and Docker (docker://…) actions, so that commands like
// `ls` can report them. Non-pinnable references are filtered out later, at
// resolution time (see isPinnableRef).
func appendUses(actions []Action, filePath string, uses *yaml.Node) []Action {
	if uses.Value == "" {
		return actions
	}

	return append(actions, Action{FilePath: filePath, Node: *uses})
}

// isPinnableRef reports whether a `uses:` value refers to an action gh-act can
// resolve and pin. Local (./…) and Docker (docker://…) references cannot be
// pinned to a tagged release.
func isPinnableRef(value string) bool {
	return value != "" &&
		!strings.HasPrefix(value, ".") &&
		!strings.HasPrefix(value, "docker://")
}

// mappingValue returns the value node associated with key in a YAML mapping
// node, or nil if the key is absent or node is not a mapping.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}

	return nil
}
