package cmd

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:               "logs",
	Aliases:           []string{"log"},
	Short:             "Interact with logs",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		fmt.Println(cfg.LogPath)

		return nil
	},
}

var listLogsCmd = &cobra.Command{
	Use:               "list",
	Aliases:           []string{"ls"},
	Short:             "List log files",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		files, err := filepath.Glob(filepath.Join(cfg.LogPath, fmt.Sprintf(logNameFormat, "*")))
		if err != nil {
			return err
		}

		for _, file := range files {
			fmt.Println(file)
		}

		return nil
	},
}

var viewLogsCmd = &cobra.Command{
	Use:               "view",
	Aliases:           []string{"open", "code"},
	Short:             "View log files in VsCode",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		path, err := exec.LookPath("code")
		if err != nil {
			return fmt.Errorf("can't find VsCode app (code) in the path")
		}

		out, err := exec.Command(path, cfg.LogPath).CombinedOutput()
		output := string(out)
		if output != "" {
			fmt.Println(output)
		}
		return err
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(listLogsCmd)
	logsCmd.AddCommand(viewLogsCmd)
}
