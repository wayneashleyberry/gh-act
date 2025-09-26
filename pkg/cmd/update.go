package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func UpdateActions(ctx context.Context, pin bool) error {
	files, err := findYAMFiles()
	if err != nil {
		return fmt.Errorf("find yaml files: %w", err)
	}

	for _, filepath := range files {
		slog.Debug("get updates for file", slog.String("file.path", filepath))

		updates, err := parseActionsInFile(ctx, filepath)
		if err != nil {
			return fmt.Errorf("get updates for file: %w", err)
		}

		slog.Debug("update file", slog.String("file.path", filepath), slog.Int("action.count", len(updates)))

		if pin {
			for n, update := range updates {
				update.VersionStyle = PinnedVersion
				updates[n] = update
			}
		}

		err = updateFile(filepath, updates)
		if err != nil {
			return fmt.Errorf("update file: %w", err)
		}
	}

	return nil
}

func updateFile(filepath string, updates []ParsedAction) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("read file: %s: %w", filepath, err)
	}

	lines := strings.Split(string(data), "\n")

	for _, update := range updates {
		slog.Debug("updating action", slog.String("action", update.ActionReference()), slog.String("file.path", filepath))

		i := update.Node.Line - 1

		line := lines[i]

		before, _, found := strings.Cut(line, "uses:")
		if !found {
			return errors.New("line separator not found")
		}

		if update.VersionStyle == PinnedVersion {
			line = before + fmt.Sprintf("uses: %s@%s # %s", update.ActionReference(), update.LatestVersionTag.Commit.GetSHA(), update.LatestVersionTag.GetName())
		} else {
			newVersionString, err := update.NewVersionString()
			if err != nil {
				return fmt.Errorf("new version from latest version tag: %w", err)
			}

			// Always add a comment with the new version tag name when updating
			line = before + fmt.Sprintf("uses: %s@%s # %s", update.ActionReference(), newVersionString, update.LatestVersionTag.GetName())
		}

		lines[i] = line
	}

	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("create file: %s: %w", filepath, err)
	}

	_, err = f.WriteString(strings.Join(lines, "\n"))
	if err != nil {
		return fmt.Errorf("write string to file: %s: %w", filepath, err)
	}

	return nil
}
