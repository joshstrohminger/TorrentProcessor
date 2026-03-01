package menu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/caseymrm/menuet"
	"github.com/joshstrohminger/TorrentProcessor/internal/api"
	"github.com/joshstrohminger/TorrentProcessor/internal/app"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/joshstrohminger/TorrentProcessor/internal/daemon"
	"github.com/joshstrohminger/TorrentProcessor/internal/logs"
	"google.golang.org/grpc"
)

const debounce = 5 * time.Second
const debounceLimit = time.Minute
const historyLengthLimit = 20

type HistoryItem struct {
	When   time.Time `json:"when"`
	Reason string    `json:"reason"`
}

type RunData struct {
	LastLogTime time.Time      `json:"lastLogTime,omitempty"`
	History     []*HistoryItem `json:"history,omitempty"`
}

func (r *RunData) filename() string {
	return filepath.Join(config.GetUserAppConfigDir(), "run-data.json")
}

func (r *RunData) load() error {
	path := r.filename()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// no file, must be initial run
			return nil
		}
		return fmt.Errorf("failed to read run data from %s: %w", path, err)
	}

	if err := json.Unmarshal(data, r); err != nil {
		return fmt.Errorf("failed to parse run data from %s: %w", path, err)
	}

	return nil
}

func (r *RunData) save() error {
	path := r.filename()

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal run data to JSON: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write run data to %s: %w", path, err)
	}

	return nil
}

type Status struct {
	updated     time.Time
	queued      int
	running     bool
	infoLogs    int
	warningLogs int
	errorLogs   int
}

type Server struct {
	api.UnimplementedControlServiceServer

	config config.App
	logger *slog.Logger

	started time.Time
	runData RunData
	refresh chan *api.RefreshRequest
	status  Status

	mu         sync.Mutex
	grpcServer *grpc.Server
}

func NewServer(config config.App, logger *slog.Logger) *Server {
	return &Server{
		config:  config,
		logger:  logger,
		started: time.Now(),
		refresh: make(chan *api.RefreshRequest),
	}
}

func (s *Server) update(req *api.RefreshRequest) {
	if req != nil {
		item := &HistoryItem{
			When:   req.GetWhen().AsTime(),
			Reason: req.GetReason().String(),
		}
		if len(s.runData.History) >= historyLengthLimit {
			s.runData.History = append(s.runData.History[len(s.runData.History)-historyLengthLimit:], item)
		} else {
			s.runData.History = append(s.runData.History, item)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = Status{updated: time.Now()}

	levels, err := logs.CountLevels(s.config, s.runData.LastLogTime)
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to log counts", slog.Any("error", err))
		return
	}

	for level, count := range levels {
		switch level {
		case slog.LevelDebug:
			//ignore
		case slog.LevelInfo:
			s.status.infoLogs += count
		case slog.LevelWarn:
			s.status.warningLogs += count
		case slog.LevelError:
			s.status.errorLogs += count
		default:
			s.status.warningLogs += 1
			s.logger.LogAttrs(context.Background(), slog.LevelWarn, "Unhandled log level for status", slog.String("level", level.String()), slog.Int("level-int", int(level)))
		}
	}

	s.status.queued, err = logs.CountQueued(s.config)
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to get queued count", slog.Any("error", err))
		return
	}

	info, err := daemon.GetInfo()
	if err != nil {
		s.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to get daemon status", slog.Any("error", err))
		return
	}
	s.status.running = info.Running
}

func (s *Server) Run() error {
	if err := s.runData.load(); err != nil {
		return err
	}

	debounceTimer := time.NewTimer(time.Minute)
	debounceTimer.Stop()
	var firstBounce time.Time

	save := func() {
		if err := s.runData.save(); err != nil {
			s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to save run data", slog.Any("error", err))
		}
		firstBounce = time.Time{}
		debounceTimer.Stop()
	}

	viewLogs := func() {
		if err := logs.View(s.config); err != nil {
			s.logger.LogAttrs(context.Background(), slog.LevelError, "failed to view logs", slog.Any("error", err))
		}
	}

	label, err := getAppLabel()
	if err != nil {
		return err
	}
	menuet.App().Label = label + ".menu"
	menuet.App().SetMenuState(&menuet.MenuState{Title: app.LongName, Image: "logo"})
	menuet.App().Children = func() []menuet.MenuItem {
		s.mu.Lock()
		defer s.mu.Unlock()

		refreshItem := menuet.MenuItem{Text: "Refresh", Clicked: func() {
			s.refresh <- nil
		}}

		if s.status.updated.IsZero() {
			return []menuet.MenuItem{
				{Text: "Updated: never"},
				refreshItem,
			}
		}

		running := "idle"
		if s.status.running {
			running = "running"
		}

		since := "forever"
		if !s.runData.LastLogTime.IsZero() {
			since = s.runData.LastLogTime.Format(app.TimeFormat)
		}

		warnWeight := menuet.FontWeight(menuet.WeightRegular)
		if s.status.warningLogs > 0 {
			warnWeight = menuet.WeightSemibold
		}

		errorWeight := menuet.FontWeight(menuet.WeightRegular)
		if s.status.errorLogs > 0 {
			errorWeight = menuet.WeightBlack
		}

		items := []menuet.MenuItem{
			{Text: fmt.Sprintf("Updated: %s", s.status.updated.Format(app.TimeFormat))},
			refreshItem,
			{Type: menuet.Separator},
			{Text: fmt.Sprintf("Daemon: %s", running)},
			{Text: fmt.Sprintf("Queued: %d", s.status.queued)},
			{Type: menuet.Separator},
			{Text: fmt.Sprintf("Since: %s", since)},
			{Text: fmt.Sprintf("Info: %d", s.status.infoLogs), Clicked: viewLogs},
			{Text: fmt.Sprintf("Warning: %d", s.status.warningLogs), Clicked: viewLogs, FontWeight: warnWeight},
			{Text: fmt.Sprintf("Error: %d", s.status.errorLogs), Clicked: viewLogs, FontWeight: errorWeight},
			{Text: "Ignore", Clicked: func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.runData.LastLogTime = time.Now()
				save()
				s.refresh <- nil
			}},
		}
		if len(s.runData.History) > 0 {
			items = slices.Grow(items, len(s.runData.History)+2)
			items = append(items, menuet.MenuItem{Type: menuet.Separator}, menuet.MenuItem{Text: "History"})
			for _, history := range s.runData.History {
				items = append(items, menuet.MenuItem{Text: fmt.Sprintf("%s: %s", history.Reason, history.When.Format(app.TimeFormat))})
			}
		}
		return items
	}

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
			s.logger.LogAttrs(context.Background(), slog.LevelError, "gRPC server failed", slog.Any("error", err))
		}
		s.mu.Lock()
		defer s.mu.Unlock()

		s.grpcServer = nil
	}()

	wg, ctx := menuet.App().GracefulShutdownHandles()
	wg.Add(1)

	go func() {
		defer wg.Done()

		s.update(nil)

		for {
			select {
			case <-ctx.Done():
				if !firstBounce.IsZero() {
					save()
				}
				s.stop()
				return

			case <-debounceTimer.C:
				save()

			case req := <-s.refresh:
				s.update(req)
				if req == nil {
					continue
				}

				if firstBounce.IsZero() {
					firstBounce = time.Now()
					debounceTimer.Reset(debounce)
				} else if time.Since(firstBounce) > debounceLimit {
					save()
				} else {
					debounceTimer.Reset(debounce)
				}

				menuet.App().Notification(menuet.Notification{
					Identifier:   req.GetHash(),
					Title:        req.GetReason().String(),
					Subtitle:     req.GetName(),
					Message:      req.GetWhen().AsTime().Format(app.TimeFormat),
					ActionButton: "Logs",
				})
			}
		}
	}()

	menuet.App().NotificationResponder = func(id, response string) {
		if response == "" && viewLogs != nil {
			viewLogs()
		}
	}

	menuet.App().RunApplication()
	return nil
}

func (s *Server) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.grpcServer == nil {
		return
	}

	ctx, timeoutCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer timeoutCancel()

	stopped := make(chan struct{})

	go func() {
		s.logger.Debug("Stopping gRPC server")
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		s.logger.Debug("Force stopping gRPC server")
		s.grpcServer.Stop()
	case <-stopped:
	}
}

func (s *Server) Refresh(ctx context.Context, req *api.RefreshRequest) (*api.Empty, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.refresh <- req:
		return nil, nil
	}
}

func getAppLabel() (string, error) {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("failed to read build info")
	}

	parts := strings.Split(buildInfo.Path, "/")
	hostParts := strings.Split(parts[0], ".")
	slices.Reverse(hostParts)
	parts = slices.Concat(hostParts, parts[1:])
	return strings.Join(parts, "."), nil
}
