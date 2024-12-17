package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
	"github.com/joshstrohminger/TorrentProcessor/internal/work"
	"github.com/spf13/cobra"
)

var processCmd = &cobra.Command{
	Use:          "process",
	Short:        "Process queued torrents",
	Long:         "Process completed torrents from the work.",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if appCfg, err := getAppConfig(cmd); err != nil {
			return err
		} else if limit, err := cmd.Flags().GetInt("limit"); err != nil {
			return err
		} else if dryRun, err := cmd.Flags().GetBool("dry-run"); err != nil {
			return err
		} else if work, err := work.New(appCfg.WorkPath, logger); err != nil {
			return fmt.Errorf("failed to create work list: %w", err)
		} else {
			cfg := config.Process{
				App:    appCfg,
				DryRun: dryRun,
				Limit:  limit,
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer cancel()

			flagPath := getSetupFlagPath()
			if _, err := os.Stat(flagPath); err == nil {
				logger.Debug("Found setup flag file")

				if err := writeToAllPaths(appCfg); err != nil {
					return err
				}

				if err := os.Remove(flagPath); err == nil {
					return fmt.Errorf("failed to remove setup flag file %s: %w", flagPath, err)
				}

				logger.Debug("Removed setup flag file")
			}

			if err := processWork(ctx, work, cfg); err != nil {
				return fmt.Errorf("failed to process work: %w", err)
			}
		}
		return nil
	},
}

func init() {
	processCmd.Flags().Int("limit", 0, "Limit the number of entries processed before exiting. -1 to run indefinitely, 0 to run until no more entries are found.")
	processCmd.Flags().Bool("dry-run", false, "Don't move files or entries, just log what would be done.")
	rootCmd.AddCommand(processCmd)
}

func processWork(ctx context.Context, w *work.Work, cfg config.Process) error {
	logger.Info("Watching")
	delays := []time.Duration{
		time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
	}
	retries := 0
	processor := torrent.NewProcessor(cfg, logger)

	doneHandler := w.Remove
	if cfg.DryRun {
		doneHandler = func(entry torrent.Entry) error {
			w.Ignore(entry, nil)
			return nil
		}
	}

	for {
		if entry, err := w.Next(cfg.MaxRetries >= 0 && retries >= cfg.MaxRetries); err != nil {
			var errParse work.ErrParse
			var errIgnored work.ErrIgnored

			if errors.As(err, &errParse) {
				if cfg.MaxRetries < 0 || retries < cfg.MaxRetries {
					var delay time.Duration
					if retries > len(delays)-1 {
						delay = delays[len(delays)-1]
					} else {
						delay = delays[retries]
					}
					retries++
					logger.LogAttrs(ctx, slog.LevelWarn, "Failed to get next work entry", slog.Any("error", err), slog.Int("attempt", retries), slog.Int("max", cfg.MaxRetries), slog.Duration("delay", delay))

					select {
					case <-time.After(delay):
						continue
					case <-ctx.Done():
						return nil
					}
				}
				err = fmt.Errorf("exceeded %d retries: %w", cfg.MaxRetries, err)
			} else if errors.As(err, &errIgnored) {
				// go to the next loop iteration, no need for a delay when ignoring a repeatedly failed entry
				continue
			}
			return fmt.Errorf("failed to get next work entry: %w", err)
		} else {
			retries = 0
			if entry == nil {
				if cfg.Limit == 0 {
					// done processing all available entries
					logger.Info("Nothing left to process")
					return nil
				}
				select {
				case <-time.After(cfg.DormantPeriod):
					continue
				case <-ctx.Done():
					return nil
				}
			} else if err = processor.Process(ctx, *entry); err != nil {
				w.Ignore(*entry, fmt.Errorf("failed to process entry %#v, ignoring until restart: %w", entry, err))
			} else if err = doneHandler(*entry); err != nil {
				return fmt.Errorf("failed to remove entry %#v: %w", entry, err)
			} else {
				logger.LogAttrs(ctx, slog.LevelInfo, "Success", slog.Any("entry", entry))
				if cfg.Limit > 0 {
					cfg.Limit--
					if cfg.Limit == 0 {
						logger.Debug("Limit reached")
						return nil
					}
				}
			}
		}
	}
}
