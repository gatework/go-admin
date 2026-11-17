package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gatework/motor/common/buildinfo"
)

// NewCommand builds the `version` command, which prints the build version.
func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Get version info",
		Example: "motor version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Version)
		},
	}
}
