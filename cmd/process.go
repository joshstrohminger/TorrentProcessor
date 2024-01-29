package cmd

import (
	"context"
	"errors"
	"fmt"
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
		if workPath, err := getWorkPath(cmd); err != nil {
			return err
		} else if maxRetries, err := cmd.Flags().GetInt("retries"); err != nil {
			return err
		} else if limit, err := cmd.Flags().GetInt("limit"); err != nil {
			return err
		} else if work, err := work.New(workPath); err != nil {
			return fmt.Errorf("failed to create work list: %w", err)
		} else {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer cancel()
			if err := processWork(ctx, work, maxRetries, limit); err != nil {
				return fmt.Errorf("failed to process work: %w", err)
			}
		}
		return nil
	},
}

func init() {
	processCmd.Flags().Int("retries", 3, "Maximum number of retries when trying to read the next entry.")
	processCmd.Flags().Int("limit", -1, "Limit the number of entries processed before exiting.")
	rootCmd.AddCommand(processCmd)
}

func processWork(ctx context.Context, w *work.Work, maxRetries int, limit int) error {
	delays := []time.Duration{
		time.Second,
		2 * time.Second,
		5 * time.Second,
		10 * time.Second,
		30 * time.Second,
	}
	retries := 0

	for {
		if entry, err := w.Next(); err != nil {
			var errParse work.ErrParse
			if errors.As(err, &errParse) {
				if maxRetries < 0 || retries < maxRetries {
					var delay time.Duration
					if retries > len(delays)-1 {
						delay = delays[len(delays)-1]
					} else {
						delay = delays[retries]
					}
					retries++
					logger.LogAttrs(ctx, slog.LevelWarn, "Failed to get next work entry", slog.Any("error", err), slog.Int("attempt", retries), slog.Int("max", maxRetries), slog.Duration("delay", delay))

					select {
					case <-time.After(delay):
						continue
					case <-ctx.Done():
						return nil
					}
				}
				err = fmt.Errorf("exceeded %d retries: %w", maxRetries, err)
			}
			return fmt.Errorf("failed to get next work entry: %w", err)
		} else if entry == nil {
			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return nil
			}
		} else if err = processEntry(ctx, *entry); err != nil {
			return fmt.Errorf("failed to process entry %#v: %w", entry, err)
		} else if err = w.Remove(*entry); err != nil {
			return fmt.Errorf("failed to remove entry %#v: %w", entry, err)
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "Success", slog.Any("entry", entry))
			if limit > 0 {
				limit--
				if limit == 0 {
					logger.LogAttrs(ctx, slog.LevelDebug, "Limit reached")
					return nil
				}
			}
		}
	}
}

func processEntry(ctx context.Context, entry torrent.Entry) error {
	logger.LogAttrs(ctx, slog.LevelInfo, "Processing", slog.Any("entry", entry))
	return torrent.Process(ctx, entry)
}
