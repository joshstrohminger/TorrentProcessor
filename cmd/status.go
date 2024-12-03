package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:               "status",
	Short:             "Status of pending work",
	SilenceUsage:      true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		appCfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		entries, err := os.ReadDir(appCfg.WorkPath)
		if err != nil {
			return fmt.Errorf("failed to read from work path %s: %w", appCfg.WorkPath, err)
		}

		fmt.Println(len(entries), "files in the work queue")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
