package cmd

import (
	"fmt"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/mitchellh/mapstructure"
	slogmulti "github.com/samber/slog-multi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
	"log/slog"
	"os"
	"path"
	"time"
)

var logger *slog.Logger

var rootCmd = &cobra.Command{
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		var filename string
		if dir, err := os.Executable(); err != nil {
			filename = path.Join(dir, "torrent-processor-"+cmd.Use+".log")
		}

		logger = slog.New(slogmulti.Fanout(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
			slog.NewJSONHandler(&lumberjack.Logger{
				Filename:  filename,
				MaxSize:   5,
				LocalTime: true,
				MaxAge:    365,
			}, &slog.HandlerOptions{Level: slog.LevelDebug}))).
			With(slog.String("cmd", cmd.Use))

		slog.SetDefault(logger)
	},
}

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

	if path.Ext(configPath) != "" {
		viper.SetConfigFile(configPath)
	} else {
		if configPath == "" {
			configPath = "config"
		}
		viper.SetConfigName(configPath)
		if dir, err := os.Executable(); err != nil {
			viper.AddConfigPath(dir)
		} else {
			viper.AddConfigPath("./")
		}
	}

	// set default config values
	cfg.MaxRetries = 5
	cfg.DormantPeriod = 30 * time.Second

	if err = viper.ReadInConfig(); err != nil {
		err = fmt.Errorf("failed to read config file %s: %w", configPath, err)
	} else if err = viper.Unmarshal(&cfg, viper.DecodeHook(mapstructure.StringToTimeDurationHookFunc())); err != nil {
		err = fmt.Errorf("failed to unmarshal config: %w", err)
	} else if err = cfg.Validate(); err != nil {
		err = fmt.Errorf("invalid app config: %w", err)
	}

	return
}

func init() {
	cobra.EnableCaseInsensitive = true
	rootCmd.PersistentFlags().String("config", "", "Path to the config file to use.")
}
