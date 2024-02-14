package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
	"github.com/joshstrohminger/TorrentProcessor/internal/work"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
	"os/signal"
	"time"
)

var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Process queued torrents",
	Long:  "Process completed torrents from the work.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if appCfg, err := getAppConfig(cmd); err != nil {
			return err
		} else if limit, err := cmd.Flags().GetInt("limit"); err != nil {
			return err
		} else if dryRun, err := cmd.Flags().GetBool("dry-run"); err != nil {
			return err
		} else if work, err := work.New(appCfg.WorkPath); err != nil {
			return fmt.Errorf("failed to create work list: %w", err)
		} else {
			cfg := config.Process{
				App:    appCfg,
				DryRun: dryRun,
				Limit:  limit,
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer cancel()
			if err := processWork(ctx, work, cfg); err != nil {
				return fmt.Errorf("failed to process work: %w", err)
			}
		}
		return nil
	},
}

func init() {
	processCmd.Flags().Int("limit", -1, "Limit the number of entries processed before exiting.")
	processCmd.Flags().Bool("dry-run", false, "Don't move files or entries, just log what would be done.")
	rootCmd.AddCommand(processCmd)
}

func processWork(ctx context.Context, w *work.Work, cfg config.Process) error {
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
			w.Ignore(entry)
			return nil
		}
	}

	for {
		if entry, err := w.Next(); err != nil {
			var errParse work.ErrParse
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
			}
			return fmt.Errorf("failed to get next work entry: %w", err)
		} else {
			retries = 0
			if entry == nil {
				select {
				case <-time.After(cfg.DormantPeriod):
					continue
				case <-ctx.Done():
					return nil
				}
			} else if err = processor.Process(ctx, *entry); err != nil {
				w.Ignore(*entry)
				return fmt.Errorf("failed to process entry %#v, ignoring until restart: %w", entry, err)
			} else if err = doneHandler(*entry); err != nil {
				return fmt.Errorf("failed to remove entry %#v: %w", entry, err)
			} else {
				logger.LogAttrs(ctx, slog.LevelInfo, "Success", slog.Any("entry", entry))
				if cfg.Limit > 0 {
					cfg.Limit--
					if cfg.Limit == 0 {
						logger.LogAttrs(ctx, slog.LevelDebug, "Limit reached")
						return nil
					}
				}
			}
		}
	}
}
