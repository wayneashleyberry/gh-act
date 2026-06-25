package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/wayneashleyberry/gh-act/pkg/api"
)

// applyRewrite resolves every action in the repository and rewrites each file
// according to mode. It is shared by the pin and update commands.
func applyRewrite(ctx context.Context, mode rewriteMode, dryRun bool) error {
	client, err := api.NewClient()
	if err != nil {
		return fmt.Errorf("create github client: %w", err)
	}

	files, refs, err := collectActionRefs()
	if err != nil {
		return err
	}

	actions, err := resolveActions(ctx, refs, client)
	if err != nil {
		return err
	}

	byFile := make(map[string][]ParsedAction, len(files))
	for _, action := range actions {
		byFile[action.FilePath] = append(byFile[action.FilePath], action)
	}

	for _, filePath := range files {
		slog.Debug("rewriting file", slog.String("file.path", filePath), slog.Int("action.count", len(byFile[filePath])))

		if err := rewriteFile(filePath, byFile[filePath], mode, dryRun); err != nil {
			return fmt.Errorf("rewrite %q: %w", filePath, err)
		}
	}

	return nil
}

// rewriteMode selects how an action reference is rewritten in place.
type rewriteMode int

const (
	// modePin pins each action to the SHA of its currently resolved version.
	modePin rewriteMode = iota
	// modeUpdate updates each action to the latest version, preserving its
	// original style (semver or pinned).
	modeUpdate
	// modeUpdatePin updates each action to the latest version and pins it to a
	// SHA.
	modeUpdatePin
)

// errNoResolvedTag indicates an action could not be rewritten because the
// required tag was never resolved (for example a repo with only pre-releases).
var errNoResolvedTag = errors.New("no resolved tag available for action")

// usesString builds the `uses: owner/repo@ref # comment` text for an action in
// the given mode. It never dereferences a nil tag.
func usesString(action ParsedAction, mode rewriteMode) (string, error) {
	switch mode {
	case modeUpdatePin:
		return pinnedUses(action, action.LatestVersionTag)
	case modePin:
		if action.VersionStyle == PinnedVersion {
			return pinnedUses(action, action.CurrentVersionTag)
		}

		return pinnedUses(action, action.PinVersionTag)
	case modeUpdate:
		if action.VersionStyle == PinnedVersion {
			return pinnedUses(action, action.LatestVersionTag)
		}

		newVersion, err := action.NewVersionString()
		if err != nil {
			return "", fmt.Errorf("new version string: %w", err)
		}

		return fmt.Sprintf("uses: %s@%s # %s", action.ActionReference(), newVersion, action.LatestVersionTag.GetName()), nil
	default:
		return "", fmt.Errorf("unsupported rewrite mode: %d", mode)
	}
}

func pinnedUses(action ParsedAction, tag *api.Tag) (string, error) {
	if tag == nil {
		return "", fmt.Errorf("%w: %s", errNoResolvedTag, action.ActionReference())
	}

	return fmt.Sprintf("uses: %s@%s # %s", action.ActionReference(), tag.Commit.GetSHA(), tag.GetName()), nil
}

// rewriteFile applies the requested rewrite to every action in a single file.
// When dryRun is true it prints a diff instead of writing. Writes are atomic
// and preserve the file's original line endings and trailing newline.
func rewriteFile(filePath string, actions []ParsedAction, mode rewriteMode, dryRun bool) error {
	if len(actions) == 0 {
		return nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file %q: %w", filePath, err)
	}

	original := string(data)
	lines := strings.Split(original, "\n")

	changed := false

	for _, action := range actions {
		index := action.Node.Line - 1
		if index < 0 || index >= len(lines) {
			return fmt.Errorf("line %d out of range in %q", action.Node.Line, filePath)
		}

		line := lines[index]

		carriageReturn := strings.HasSuffix(line, "\r")
		if carriageReturn {
			line = strings.TrimSuffix(line, "\r")
		}

		before, _, found := strings.Cut(line, "uses:")
		if !found {
			return fmt.Errorf("no 'uses:' found on line %d of %q", action.Node.Line, filePath)
		}

		uses, err := usesString(action, mode)
		if err != nil {
			// Skip actions we cannot safely rewrite rather than corrupt the file.
			slog.Debug("skipping action during rewrite", slog.String("action", action.ActionReference()), slog.String("error.message", err.Error()))

			continue
		}

		newLine := before + uses
		if carriageReturn {
			newLine += "\r"
		}

		if newLine != lines[index] {
			lines[index] = newLine
			changed = true
		}
	}

	if !changed {
		return nil
	}

	updated := strings.Join(lines, "\n")

	if dryRun {
		printDiff(filePath, original, updated)

		return nil
	}

	if err := atomicWriteFile(filePath, []byte(updated)); err != nil {
		return fmt.Errorf("write file %q: %w", filePath, err)
	}

	return nil
}

// atomicWriteFile writes data to a temporary file in the same directory and
// renames it over path, so a failed write can never leave a partial file.
func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".gh-act-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpName := tmp.Name()

	// Best-effort cleanup if we fail before the rename succeeds.
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmpName, info.Mode())
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// printDiff writes a minimal line-oriented diff of the changes to stdout.
func printDiff(filePath, original, updated string) {
	originalLines := strings.Split(original, "\n")
	updatedLines := strings.Split(updated, "\n")

	fmt.Printf("%s\n", filePath)

	for i := range originalLines {
		if i >= len(updatedLines) {
			break
		}

		if originalLines[i] == updatedLines[i] {
			continue
		}

		fmt.Printf("  %d:\n", i+1)
		fmt.Printf("  - %s\n", strings.TrimSuffix(originalLines[i], "\r"))
		fmt.Printf("  + %s\n", strings.TrimSuffix(updatedLines[i], "\r"))
	}
}
