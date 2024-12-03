package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"text/template"

	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/spf13/cobra"
)

//go:embed daemon.plist.tmpl
var plistTemplate string

var daemonCmd = &cobra.Command{
	Use:               "daemon",
	Aliases:           []string{"service"},
	Short:             "Control the processing daemon",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	SilenceUsage:      true,

	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := getDaemonInfo()
		if err != nil {
			return err
		}

		summary := fmt.Sprintf("%+v", info)
		summary = strings.Trim(summary, "{}")
		summary = strings.ReplaceAll(summary, " ", "\n")
		summary = strings.ReplaceAll(summary, ":", ": ")
		fmt.Println(summary)

		return printWorkQueueCount(cmd)
	},
}

var daemonStartCmd = &cobra.Command{
	Use:               "start",
	Aliases:           []string{"install", "load", "bootstrap"},
	Short:             "Install and bootstrap (start) the daemon",
	SilenceUsage:      true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {

		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		info, err := getDaemonInfo()
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
		} else if err := installPlist(info, cfg); err != nil {
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
	Use:               "stop",
	Aliases:           []string{"remove", "unload", "bootout"},
	Short:             "Disable and remove the daemon",
	SilenceUsage:      true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		info, err := getDaemonInfo()
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

func installPlist(info DaemonInfo, cfg config.App) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse plist template: %w", err)
	}

	file, err := os.OpenFile(info.Path, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open %s for write: %w", info.Path, err)
	}
	defer file.Close()

	if err = tmpl.Execute(file, TemplateData{exe, info, cfg}); err != nil {
		return fmt.Errorf("failed to execute plist template: %w", err)
	}

	return nil
}

type TemplateData struct {
	Exe    string
	Info   DaemonInfo
	Config config.App
}

type DaemonInfo struct {
	Label        string
	Domain       string
	Name         string
	Path         string
	DebugLogName string
	Installed    bool
	Enabled      bool
	Running      bool
}

func getDaemonInfo() (info DaemonInfo, err error) {
	info.Label, err = getDaemonLabel()
	if err != nil {
		return
	}

	var home string
	home, err = os.UserHomeDir()
	if err != nil {
		err = fmt.Errorf("failed to get user home directory: %w", err)
		return
	}

	info.Path = filepath.Join(home, "Library", "LaunchAgents", info.Label+".plist")
	if _, err = os.Stat(info.Path); err == nil {
		info.Installed = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return
	}

	var id string
	id, err = getUserId()
	if err != nil {
		return
	}
	info.Domain = fmt.Sprintf("gui/%s", id)
	info.Name = fmt.Sprintf("%s/%s", info.Domain, info.Label)

	if info.Installed {
		info.Enabled, info.Running, err = getDaemonStatus(info.Name)
		if err != nil {
			return
		}
	}

	return
}

func getDaemonStatus(name string) (enabled bool, running bool, err error) {
	out, err := exec.Command("launchctl", "print", name).CombinedOutput()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 113 {
			// not enabled
			err = nil
			return
		}

		err = fmt.Errorf("failed to get daemon %s status: %w", name, err)
		return
	}

	enabled = true

	reg, err := regexp.Compile(`\sstate = ((:?not )?running)\s`)
	if err != nil {
		err = fmt.Errorf("failed to get daemon %s status: failed to compile regex: %w", name, err)
		return
	}

	output := string(out)
	matches := reg.FindStringSubmatch(output)
	if matches == nil {
		err = fmt.Errorf("failed to find state for daemon %s", name)
		return
	}
	state := matches[1]

	switch state {
	case "running":
		running = true
	case "not running":
	default:
		err = fmt.Errorf("unhandled state in daemon %s: %s", name, state)
		return
	}

	return
}

func getUserId() (string, error) {
	// lookup the user name in case we're running as sudo
	user, ok := os.LookupEnv("SUDO_USER")
	if !ok {
		user, ok = os.LookupEnv("USER")
		if !ok {
			return "", fmt.Errorf("failed to lookup current user via environment variables")
		}
	}

	out, err := exec.Command("id", "-u", user).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get user ID: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func getDaemonLabel() (string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("failed to get daemon label: failed to get build info")
	}

	parts := strings.Split(info.Path, "/")

	domainParts := strings.Split(parts[0], ".")
	slices.Reverse(domainParts)
	parts[0] = strings.Join(domainParts, ".")

	return strings.Join(parts, "."), nil
}

func init() {
	processCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd, daemonStopCmd)

	daemonStartCmd.Flags().Bool("force", false, "Force re-installation if it already exists")
	daemonStartCmd.Flags().Bool("debug", false, fmt.Sprintf("Enable additional launchd logging and write stdout and stderr to %s in the configured log directory", debugLogName))
}
