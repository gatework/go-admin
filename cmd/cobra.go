package cmd

import (
	"os"

	"github.com/gatework/motor/cmd/app"
	"github.com/gatework/motor/common/buildinfo"

	"github.com/spf13/cobra"

	"github.com/gatework/motor/cmd/api"
	"github.com/gatework/motor/cmd/config"
	"github.com/gatework/motor/cmd/migrate"
	"github.com/gatework/motor/cmd/version"
)

// NewRootCommand builds the motor command line with every subcommand.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:          "motor",
		Short:        "motor",
		SilenceUsage: true,
		Long:         "motor " + buildinfo.Version + " - network operations backend. See README.md and docs/contract.md.",
	}
	root.AddCommand(api.NewCommand(), migrate.NewCommand(), version.NewCommand(), config.NewCommand(), app.NewCommand())
	return root
}

// Execute runs the command line and exits non-zero when a command fails;
// cobra has already printed the error.
func Execute() {
	if err := NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
