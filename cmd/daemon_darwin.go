package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/joshstrohminger/TorrentProcessor/internal/daemon"
	"github.com/joshstrohminger/TorrentProcessor/internal/util/term"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:               "daemon",
	Aliases:           []string{"service"},
	Short:             "Control the processing daemon",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	SilenceUsage:      true,

	RunE: func(cmd *cobra.Command, args []string) error {
		exitCode, err := cmd.Flags().GetBool("exit-code")
		if err != nil {
			return err
		}

		info, err := daemon.GetInfo()
		if err != nil {
			return err
		}

		info.Queued, err = countQueued(cmd)
		if err != nil {
			return err
		}

		term.PrintStruct(info)

		if exitCode && info.Running {
			os.Exit(1)
		}

		return nil
	},
}

var daemonRunCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"trigger", "kickstart"},
	Short:   "Run the daemon now",
	Long:    "Manually trigger the daemon to run now",

	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := daemon.GetInfo()
		if err != nil {
			return err
		}

		if !info.Installed {
			return fmt.Errorf("daemon is not installed")
		}

		out, err := exec.Command("launchctl", "kickstart", info.Name).CombinedOutput()
		if err != nil {
			out = bytes.TrimSpace(out)
			if len(out) != 0 {
				return fmt.Errorf("failed to kickstart daemon: %s: %w", string(bytes.TrimSpace(out)), err)
			}
			return fmt.Errorf("failed to kickstart daemon: %w", err)
		}

		return nil
	},
}

var daemonStartCmd = &cobra.Command{
	Use:          "start",
	Aliases:      []string{"install", "load", "bootstrap"},
	Short:        "Install and bootstrap (start) the daemon",
	SilenceUsage: true,

	RunE: func(cmd *cobra.Command, args []string) error {

		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		info, err := daemon.GetInfo()
		if err != nil {
			return err
		}

		if debug, err := cmd.Flags().GetBool("debug"); err != nil {
			return err
		} else if debug {
			info.DebugLogName = debugLogName
		}

		if force, err := cmd.Flags().GetBool("force"); err != nil {
			return err
		} else if force {
			info.Installed = false
			info.Enabled = false
			info.Running = false
		}

		if info.Installed {
			fmt.Println("Daemon is already installed at", info.Path)
		} else if err := daemon.InstallPlist(info, cfg); err != nil {
			return fmt.Errorf("failed to install daemon: %w", err)
		} else {
			fmt.Println("Daemon installed at", info.Path)
		}

		bootstrapCommand := exec.Command("launchctl", "bootstrap", info.Domain, info.Path)
		if info.Enabled {
			fmt.Println("Daemon is already enabled")
		} else if out, err := bootstrapCommand.CombinedOutput(); err != nil {
			fmt.Println(bootstrapCommand)
			output := strings.TrimSpace(string(out))
			if output != "" {
				fmt.Println(string(out))
			}
			return err
		} else {
			fmt.Println("Daemon bootstrapped")
		}

		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:          "stop",
	Aliases:      []string{"remove", "unload", "bootout"},
	Short:        "Disable and remove the daemon",
	SilenceUsage: true,

	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := daemon.GetInfo()
		if err != nil {
			return err
		}

		if !info.Enabled {
			fmt.Println("Daemon is already disabled")
		} else if err := exec.Command("launchctl", "bootout", info.Name).Run(); err != nil {
			return fmt.Errorf("failed to bootout daemon %s: %w", info.Path, err)
		} else {
			fmt.Println("Daemon disabled")
		}

		if !info.Installed {
			fmt.Println("Daemon is already not installed at", info.Path)
		} else if err := os.Remove(info.Path); err != nil {
			return fmt.Errorf("failed to remove plist %s: %w", info.Path, err)
		} else {
			fmt.Println("Plist removed at", info.Path)
		}

		return nil
	},
}

func init() {
	processCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd, daemonStopCmd, daemonRunCmd)

	daemonCmd.Flags().Bool("exit-code", false, "Set the exit code to 0 if not running, 1 if running")

	daemonStartCmd.Flags().Bool("force", false, "Force re-installation if it already exists")
	daemonStartCmd.Flags().Bool("debug", false, fmt.Sprintf("Enable additional launchd logging and write stdout and stderr to %s in the configured log directory", debugLogName))
}
