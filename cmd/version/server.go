package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gatework/motor/common/global"
)

// StartCmd holds the configured startcmd value.
var (
	StartCmd = &cobra.Command{
		Use:     "version",
		Short:   "Get version info",
		Example: "motor version",
		PreRun: func(cmd *cobra.Command, args []string) {

		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}
)

func run() error {
	fmt.Println(global.Version)
	return nil
}
