package cmd

import "fmt"

func ListActions() error {
	actions, err := findActions()
	if err != nil {
		return fmt.Errorf("find actions: %w", err)
	}

	for _, action := range actions {
		comment := ""
		if action.Node.LineComment != "" {
			comment = " " + action.Node.LineComment
		}

		fmt.Printf("%s:%d:%d: %s%s\n", action.FilePath, action.Node.Line, action.Node.Column, action.Node.Value, comment)
	}

	return nil
}

func findActions() ([]Action, error) {
	yamlFiles, err := findYAMFiles()
	if err != nil {
		return []Action{}, fmt.Errorf("find yaml files: %w", err)
	}

	var actions []Action

	for _, filePath := range yamlFiles {
		findings, err := parseYAMLFile(filePath)
		if err != nil {
			return []Action{}, err
		}

		actions = append(actions, findings...)
	}

	return actions, nil
}
