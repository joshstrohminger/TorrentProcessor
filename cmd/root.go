package cmd

import (
	"fmt"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

func getAppConfig(cmd *cobra.Command) (cfg config.App, err error) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return
	}

	viper.SetConfigFile(configPath)
	if err = viper.ReadInConfig(); err != nil {
		err = fmt.Errorf("failed to read config file %s: %w", configPath, err)
		return
	}

	if err = viper.Unmarshal(&cfg); err != nil {
		err = fmt.Errorf("failed to unmarshal config: %w", err)
	}

	err = cfg.Validate()
	return
}

func init() {
	cobra.EnableCaseInsensitive = true
	rootCmd.PersistentFlags().String("config", "", "Path to the config file to use.")
}
