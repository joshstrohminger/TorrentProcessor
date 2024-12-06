package cmd

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/joshstrohminger/TorrentProcessor/internal/app"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/joshstrohminger/TorrentProcessor/internal/util"
	"github.com/mitchellh/mapstructure"
	slogmulti "github.com/samber/slog-multi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *slog.Logger

//go:embed .version
var version string

const longDescription = `
This is intended to be run as two separate processes; one using the
'process' command which runs as a service/daemon, and one called by
the torrent client with the 'add' command. It will use a working
directory of JSON files as a queue of torrents to be processed.`

var rootCmd = &cobra.Command{
	Short:         "Utility for processing completed torrents",
	Version:       version,
	SilenceErrors: true, // we'll log errors on our own
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Name() == "help" {
			// don't do anything for the built-in help command
			return nil
		}

		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		relPath := filepath.Join(cfg.LogPath, fmt.Sprintf(logNameFormat, cmd.Name()))
		path, err := filepath.Abs(relPath)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for log file from %s: %w", relPath, err)
		}

		jsonLogger := slog.NewJSONHandler(&lumberjack.Logger{
			Filename:  path,
			MaxSize:   5,
			LocalTime: true,
			MaxAge:    365,
		}, &slog.HandlerOptions{Level: slog.LevelDebug})

		stdoutLogger := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})

		logger = slog.New(slogmulti.Fanout(stdoutLogger, jsonLogger)).With(slog.String("cmd", cmd.Use))
		slog.SetDefault(logger)

		logger.LogAttrs(cmd.Context(), slog.LevelInfo, "Parsed config", slog.Any("config", cfg), slog.String("path", viper.ConfigFileUsed()), slog.Any("args", os.Args), slog.String("version", version), slog.String("buildVersion", cmd.Root().Version))

		// Log the log path to the console only
		stdoutLogger.Handle(cmd.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "Logging to "+path, 0))
		return nil
	},
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if logger == nil {
			fmt.Println(fmt.Errorf("failed to execute: %w", err).Error())
		} else {
			logger.LogAttrs(rootCmd.Context(), slog.LevelError, "Failed to execute", slog.Any("error", err))
		}
		os.Exit(1)
	}
}

func getAppConfig(cmd *cobra.Command) (config.App, error) {
	cfg := config.Default()

	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return cfg, err
	}

	if slices.Contains(viper.SupportedExts, strings.ToLower(strings.TrimPrefix(filepath.Ext(configPath), "."))) {
		// file path provided, use it directly
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName(app.ShortName)

		if configPath != "" {
			// directory provided, search there first
			viper.AddConfigPath(configPath)
		}

		viper.AddConfigPath(config.GetUserAppConfigDir())

		if dir, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(dir)
		}

		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)

			if inTemp, err := util.IsFileBelowDir(exe, os.TempDir()); inTemp {
				// assume we've been run using "go run" and might have a config file local to the working directory
				viper.AddConfigPath(".")
			} else if err != nil {
				return cfg, fmt.Errorf("failed to check if running from the temp dir: %w", err)
			}

			viper.AddConfigPath(dir)
		}
	}

	viper.AutomaticEnv()
	viper.EnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.SetEnvPrefix(strings.ToUpper(app.ShortName))

	if err = viper.ReadInConfig(); err != nil {
		return cfg, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	def := config.Default()
	viper.Set("workpath", def.WorkPath)
	viper.Set("logpath", def.LogPath)

	if err = viper.Unmarshal(&cfg, func(decoderConfig *mapstructure.DecoderConfig) {
		decoderConfig.ErrorUnused = true
	}); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err = cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid app config: %w", err)
	}

	return cfg, nil
}

func init() {
	cobra.EnableCaseInsensitive = true
	rootCmd.PersistentFlags().String("config", "", "Path to the config file to use.")
	rootCmd.Long = rootCmd.Short + "\n" + longDescription

	if exe, err := os.Executable(); err == nil {
		const timeFormat = "Jan 2, 2006 at 3:04:05 PM"

		if info, err := os.Stat(exe); err == nil {
			rootCmd.Version = fmt.Sprintf("%s, built %s", rootCmd.Version, info.ModTime().Format(timeFormat))
		}

		if info, ok := debug.ReadBuildInfo(); ok {
			settings := make(map[string]string)
			for _, setting := range info.Settings {
				settings[setting.Key] = setting.Value
			}

			revision, foundRevision := settings["vcs.revision"]
			revisionTimeString, foundTime := settings["vcs.time"]
			dirty, foundDirty := settings["vcs.modified"]
			if foundRevision && foundTime {
				var dirtyLabel string
				if foundDirty && dirty == "true" {
					dirtyLabel = " (dirty)"
				}

				if revisionTime, err := time.Parse(time.RFC3339, revisionTimeString); err == nil {
					revisionTimeString = revisionTime.Local().Format(timeFormat)
				}

				rootCmd.Version = fmt.Sprintf("%s, from ref %s%s, committed %s", rootCmd.Version, revision, dirtyLabel, revisionTimeString)
			}
		}
	}
}
