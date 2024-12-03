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
		return printWorkQueueCount(cmd)
	},
}

func printWorkQueueCount(cmd *cobra.Command) error {
	appCfg, err := getAppConfig(cmd)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(appCfg.WorkPath)
	if err != nil {
		return fmt.Errorf("failed to read from work path %s: %w", appCfg.WorkPath, err)
	}

	fmt.Println("Queued:", len(entries))

	return nil
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
