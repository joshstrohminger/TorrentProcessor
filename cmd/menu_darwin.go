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

var menuInstallCmd = &cobra.Command{
	Use:          "install",
	Aliases:      []string{"setup", "start"},
	SilenceUsage: true,

	RunE: func(cmd *cobra.Command, args []string) error {
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}

		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		var logName string
		if debug, err := cmd.Flags().GetBool("debug"); err != nil {
			return err
		} else if debug {
			logName = debugLogName
		}

		return menu.Install(cfg, logger, version, force, logName)
	},
}

var menuUninstallCmd = &cobra.Command{
	Use:          "uninstall",
	Aliases:      []string{"remove", "stop"},
	SilenceUsage: true,

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		return menu.Uninstall(cfg, logger)
	},
}

func init() {
	menuInstallCmd.Flags().BoolP("force", "f", false, "Force re-install")
	menuInstallCmd.Flags().Bool("debug", false, "Enable debug logging")

	menuCmd.AddCommand(menuInstallCmd, menuUninstallCmd)

	rootCmd.AddCommand(menuCmd)
}
