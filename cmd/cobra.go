package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/gatework/motor-core/sdk/pkg"
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
	Long:         `motor`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			tip()
			return errors.New(pkg.Red("requires at least one arg"))
		}
		return nil
	},
	PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
	Run: func(cmd *cobra.Command, args []string) {
		tip()
	},
}

func tip() {
	usageStr := `欢迎使用 ` + pkg.Green(`motor `+buildinfo.Version) + ` 可以使用 ` + pkg.Red(`-h`) + ` 查看命令`
	usageStr1 := `配置与开发约定见仓库 README.md 和 docs/contract.md`
	fmt.Printf("%s\n", usageStr)
	fmt.Printf("%s\n", usageStr1)
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
