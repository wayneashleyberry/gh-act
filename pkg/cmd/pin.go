package cmd

import "context"

// PinActions pins every action to the commit SHA of its currently resolved
// version. When dryRun is true the changes are printed instead of written.
func PinActions(ctx context.Context, dryRun bool) error {
	return applyRewrite(ctx, modePin, dryRun)
}
