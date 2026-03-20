package check

import (
	"github.com/spf13/cobra"
)

// NewCheckCommand returns the check command (connectivity / config checks).
func NewCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run connectivity and config checks",
	}

	cmd.AddCommand(newDashScopeCommand())
	cmd.AddCommand(newGraphMemoryCommand())

	return cmd
}
