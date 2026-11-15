package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/gatework/motor-core/config/source/file"
	"github.com/gatework/motor-core/sdk/config"
)

var configYml string

// StartCmd holds the configured startcmd value.
var StartCmd = &cobra.Command{
	Use: "config", Short: "Show application configuration", Example: "motor config -c config/settings.yml",
	RunE: func(cmd *cobra.Command, args []string) error { return run(cmd.Context(), cmd.OutOrStdout()) },
}

//nolint:gochecknoinits // Registers host commands and routes before the runtime startup registry is sealed.
func init() {
	StartCmd.PersistentFlags().StringVarP(&configYml, "config", "c", "config/settings.yml", "Configuration file")
}

func run(ctx context.Context, out io.Writer) (err error) {
	watcher, err := config.Setup(ctx, file.NewSource(file.WithPath(configYml)), config.Options{})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, watcher.Close()) }()
	data, err := json.MarshalIndent(config.Current(), "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
