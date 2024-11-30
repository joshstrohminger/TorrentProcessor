package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joshstrohminger/TorrentProcessor/internal/api"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
	"github.com/joshstrohminger/TorrentProcessor/internal/work"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
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

type Server struct {
	api.UnimplementedControlServiceServer

	config  config.App
	started time.Time
	exit    chan int

	mu         sync.Mutex
	grpcServer *grpc.Server
}

func newServer(config config.App) *Server {
	return &Server{
		config:  config,
		exit:    make(chan int, 1),
		started: time.Now(),
	}
}

func (s *Server) run() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.grpcServer != nil {
		return fmt.Errorf("already running")
	}

	lis, err := net.Listen("tcp", s.config.Api.String())
	if err != nil {
		return fmt.Errorf("failed to listen to %s: %w", s.config.Api, err)
	}

	s.grpcServer = grpc.NewServer()
	api.RegisterControlServiceServer(s.grpcServer, s)

	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			logger.LogAttrs(rootCmd.Context(), slog.LevelError, "gRPC server failed", slog.Any("error", err))
		}
		s.mu.Lock()
		defer s.mu.Unlock()

		s.grpcServer = nil
	}()

	return nil
}

func (s *Server) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.grpcServer != nil {
		logger.Debug("Stopping gRPC server")

		ctx, timeoutCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer timeoutCancel()

		ctx, sigCancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		defer sigCancel()

		stopped := make(chan struct{})

		go func() {
			logger.Debug("Force stopping gRPC server")
			s.grpcServer.GracefulStop()
			close(stopped)
		}()

		select {
		case <-ctx.Done():
			s.grpcServer.Stop()
		case <-stopped:
		}
	}

	s.grpcServer = nil
}

func (s *Server) GetConfig(context.Context, *api.Empty) (*api.Config, error) {
	return &api.Config{
		WorkPath:        s.config.WorkPath,
		MovieOutputPath: s.config.MovieOutputPath,
		TvOutputPath:    s.config.TvOutputPath,
		DormantPeriod:   durationpb.New(s.config.DormantPeriod),
		MaxRetries:      uint32(s.config.MaxRetries),
	}, nil
}

func (s *Server) Check(context.Context, *api.Empty) (*api.ProcessingStatus, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Check not implemented")
}

func (s *Server) Stop(context.Context, *api.Empty) (*api.ProcessingStatus, error) {
	logger.Info("Stopping")
	s.stop()
	s.exit <- 0
	return s.status(), nil
}

func (s *Server) Restart(context.Context, *api.Empty) (*api.ProcessingStatus, error) {
	logger.Info("Restarting")
	s.stop()
	s.exit <- 1
	return s.status(), nil
}

func (s *Server) status() *api.ProcessingStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	return &api.ProcessingStatus{
		Running: s.grpcServer != nil,
	}
}
