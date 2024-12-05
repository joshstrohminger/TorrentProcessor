package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/joshstrohminger/TorrentProcessor/internal/app"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/joshstrohminger/TorrentProcessor/internal/util/term"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var setupCmd = &cobra.Command{
	Use:               "setup",
	Aliases:           []string{"init"},
	Short:             "Setup the config and permissions",
	Long:              "Ensure a configuration file exists and actions have been performed that would trigger permissions requests.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	SilenceUsage:      true,

	RunE: func(cmd *cobra.Command, args []string) error {
		queue, err := cmd.Flags().GetBool("queue")
		if err != nil {
			return err
		}

		appCfg, err := setupAppConfig(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Setup is valid:", viper.ConfigFileUsed())

		if err := writeToAllPaths(appCfg, queue); err != nil {
			return err
		}

		fmt.Println("All directories are accessible")

		return nil
	},
}

func writeToAllPaths(appCfg config.App, queue bool) error {
	if err := appCfg.VisitPaths(func(name, dir string) error {
		path := filepath.Join(dir, app.SetupTriggerName)

		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close file %s: %w", path, err)
		}

		// don't remove the file for the workpath if we need to queue setup to run again
		if !queue || dir != appCfg.WorkPath {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove file %s: %w", path, err)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to trigger permissions requests: %w", err)
	}

	return nil
}

func setupAppConfig(cmd *cobra.Command) (config.App, error) {
	for {
		viper.Reset()
		appCfg, err := getAppConfig(cmd)
		if err == nil {
			return appCfg, nil
		}
		cfgErr := err

		// create config file if it doesn't exist
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			dir, err := getUserAppCfgDir()
			if err != nil {
				return appCfg, err
			}

			path := filepath.Join(dir, app.ShortName+config.Ext)

			if _, err := os.Stat(path); err == nil {
				return appCfg, fmt.Errorf("config file %s already exists", path)
			}

			data, err := appCfg.Marshal()
			if err != nil {
				return appCfg, fmt.Errorf("failed to marshal config: %w", err)
			}

			if err := os.WriteFile(path, data, 0644); err != nil {
				return appCfg, fmt.Errorf("failed to write config to %s: %w", path, err)
			}

			fmt.Println("Created config file:", path)
			continue
		}

		// edit config file in the terminal
		path := viper.ConfigFileUsed()
		editor := term.GetEditor()

		if !term.IsInteractive() || editor == "" {
			fmt.Println("Re-run command after editing config:", path)
			return appCfg, err
		}

		fmt.Println(err)
		yes, err := term.PromptYesNo(fmt.Sprintf("Do you want to edit the config with %s?", filepath.Base(editor)))
		if err != nil {
			return appCfg, fmt.Errorf("failed to prompt to edit: %w", err)
		}
		if !yes {
			return appCfg, cfgErr
		}

		edit := exec.Command(editor, path)
		edit.Stdin = os.Stdin
		edit.Stderr = os.Stderr
		edit.Stdout = os.Stdout
		if err := edit.Run(); err != nil {
			return appCfg, fmt.Errorf("failed to edit %s: %w", path, err)
		}
	}
}

func init() {
	rootCmd.AddCommand(setupCmd)
	setupCmd.Flags().Bool("queue", false, "Queue setup again the next time the 'process' command runs")
}
