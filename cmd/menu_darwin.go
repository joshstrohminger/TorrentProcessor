package cmd

import (
	"github.com/joshstrohminger/TorrentProcessor/internal/menu"
	"github.com/spf13/cobra"
)

var menuCmd = &cobra.Command{
	Use:          "menu",
	Short:        "Control the menu bar item",
	SilenceUsage: true,

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		server := menu.NewServer(cfg, logger)
		return server.Run()
	},
}

func init() {
	rootCmd.AddCommand(menuCmd)
}
