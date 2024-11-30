package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/mitchellh/mapstructure"
	slogmulti "github.com/samber/slog-multi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

var logger *slog.Logger

const logNameFormat = "tp.%s.log"

const longDescription = `
This is intended to be run as two separate processes; one using the
'process' command which runs as a service/daemon, and one called by
the torrent client with the 'add' command. It will use a working
directory of JSON files as a queue of torrents to be processed.`

var rootCmd = &cobra.Command{
	Short:         "Utility for processing completed torrents",
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

		logger.LogAttrs(cmd.Context(), slog.LevelInfo, "Parsed config", slog.Any("config", cfg), slog.String("path", viper.ConfigFileUsed()), slog.Any("args", os.Args))

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

func getOsLogDir() (dir string, err error) {
	switch runtime.GOOS {
	case "windows":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home dir: %w", err)
		}
		return filepath.Join(home, "AppData", "Local"), nil

	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home dir: %w", err)
		}
		return filepath.Join(home, "Library", "Logs"), nil

	case "linux":
		return "/var/log/tp", nil

	default:
		return "", fmt.Errorf("unsupported GOOS %s", runtime.GOOS)
	}
}

// isFileBelowDir checks if the file is contained withing dir or one of its child directories.
func isFileBelowDir(file string, dir string) (bool, error) {
	relPath, err := filepath.Rel(dir, file)
	if err != nil {
		return false, err
	}

	// Check if the relative path starts with "..", meaning it's not within the parent directory
	return !strings.HasPrefix(relPath, ".."), nil
}

func getAppConfig(cmd *cobra.Command) (cfg config.App, err error) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return
	}

	if slices.Contains(viper.SupportedExts, strings.ToLower(strings.TrimPrefix(filepath.Ext(configPath), "."))) {
		// file path provided, use it directly
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("tp")

		if configPath != "" {
			// directory provided, search there first
			viper.AddConfigPath(configPath)
		}

		if dir, err := os.UserConfigDir(); err == nil {
			viper.AddConfigPath(filepath.Join(dir, "tp"))
		}

		if dir, err := os.UserHomeDir(); err == nil {
			viper.AddConfigPath(dir)
		}

		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)

			if inTemp, err := isFileBelowDir(exe, os.TempDir()); inTemp {
				// assume we've been run using "go run" and might have a config file local to the working directory
				viper.AddConfigPath(".")
			} else if err != nil {
				return config.App{}, fmt.Errorf("failed to check if running from the temp dir: %w", err)
			}

			viper.AddConfigPath(dir)
		}
	}

	viper.AutomaticEnv()
	viper.EnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.SetEnvPrefix("TP")

	if err = viper.ReadInConfig(); err != nil {
		err = fmt.Errorf("failed to read config file %s: %w", configPath, err)
		return
	}

	if err = viper.Unmarshal(&cfg, func(decoderConfig *mapstructure.DecoderConfig) {
		decoderConfig.ErrorUnused = true
	}); err != nil {
		err = fmt.Errorf("failed to unmarshal config: %w", err)
		return
	}

	if cfg.LogPath == "" {
		cfg.LogPath, err = getOsLogDir()
		if err != nil {
			return
		}
	}

	if err = cfg.Validate(); err != nil {
		err = fmt.Errorf("invalid app config: %w", err)
		return
	}

	return
}

func init() {
	cobra.EnableCaseInsensitive = true
	rootCmd.PersistentFlags().String("config", "", "Path to the config file to use.")
	rootCmd.Long = rootCmd.Short + "\n" + longDescription
}
