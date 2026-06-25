package cmd

import "context"

// UpdateActions updates every action to its latest version. When pin is true
// each action is additionally pinned to a commit SHA, which is also required to
// resolve branch references such as @main. When dryRun is true the changes are
// printed instead of written.
func UpdateActions(ctx context.Context, pin, dryRun bool) error {
	mode := modeUpdate
	if pin {
		mode = modeUpdatePin
	}

	return applyRewrite(ctx, mode, dryRun)
}
