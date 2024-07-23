package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func PinActions(ctx context.Context) error {
	files, err := findYAMFiles()
	if err != nil {
		return fmt.Errorf("find yaml files: %w", err)
	}

	for _, filepath := range files {
		slog.Debug("get updates for file", slog.String("file.path", filepath))

		actions, err := parseActionsInFile(ctx, filepath)
		if err != nil {
			return fmt.Errorf("get updates for file: %s: %w", filepath, err)
		}

		slog.Debug("pin file", slog.String("file.path", filepath))

		err = pinFile(filepath, actions)
		if err != nil {
			return fmt.Errorf("pin file: %w", err)
		}
	}

	return nil
}

func pinFile(filepath string, updates []ParsedAction) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("read file: %s: %w", filepath, err)
	}

	lines := strings.Split(string(data), "\n")

	for _, update := range updates {
		i := update.Node.Line - 1

		line := lines[i]

		before, _, found := strings.Cut(line, "uses:")
		if !found {
			return fmt.Errorf("line separator not found")
		}

		if update.VersionStyle == PinnedVersion {
			line = before + fmt.Sprintf("uses: %s/%s@%s # %s", update.Owner, update.Repo, update.CurrentVersionTag.Commit.GetSHA(), update.CurrentVersionTag.GetName())
		} else {
			line = before + fmt.Sprintf("uses: %s/%s@%s # %s", update.Owner, update.Repo, update.PinVersionTag.Commit.GetSHA(), update.PinVersionTag.GetName())
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
