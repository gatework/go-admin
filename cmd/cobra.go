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

var rootCmd = &cobra.Command{
	Use:          "motor",
	Short:        "motor",
	SilenceUsage: true,
	Long:         "motor " + buildinfo.Version + " - network operations backend. See README.md and docs/contract.md.",
}

//nolint:gochecknoinits // Registers host commands and routes before the runtime startup registry is sealed.
func init() {
	rootCmd.AddCommand(api.StartCmd)
	rootCmd.AddCommand(migrate.StartCmd)
	rootCmd.AddCommand(version.StartCmd)
	rootCmd.AddCommand(config.StartCmd)
	rootCmd.AddCommand(app.StartCmd)
}

// Execute : apply commands
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
