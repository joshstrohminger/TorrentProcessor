package cmd

import (
	"github.com/spf13/cobra"
	"log/slog"
	"os"
)

var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

var rootCmd = &cobra.Command{}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logger.LogAttrs(rootCmd.Context(), slog.LevelError, "Failed to execute", slog.Any("error", err))
		os.Exit(1)
	}
}

func getWorkPath(cmd *cobra.Command) (string, error) {
	return cmd.Flags().GetString("work")
}

func init() {
	cobra.EnableCaseInsensitive = true
	rootCmd.PersistentFlags().StringP("work", "w", "work.json", "Path to the JSON work directory to use.")
}
