package menu

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/joshstrohminger/TorrentProcessor/internal/api"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Client struct {
	conn   *grpc.ClientConn
	client api.ControlServiceClient
	logger *slog.Logger
}

func NewClient(cfg config.App, logger *slog.Logger) *Client {
	c := &Client{
		logger: logger,
	}

	var err error
	if c.conn, err = grpc.NewClient(cfg.Api.String(), grpc.WithTransportCredentials(insecure.NewCredentials())); err != nil {
		logger.LogAttrs(context.Background(), slog.LevelError, "failed to create gRPC client", slog.Any("error", fmt.Errorf("failed to create gRPC client for %s: %w", cfg.Api, err)))
	} else {
		c.client = api.NewControlServiceClient(c.conn)
	}

	return c
}

func (c *Client) Refresh(ctx context.Context, reason api.RefreshRequest_Reason) {
	if c.client == nil {
		// we must have failed to create the client
		return
	}

	req := new(api.RefreshRequest)
	req.SetWhen(timestamppb.Now())
	req.SetReason(reason)

	if _, err := c.client.Refresh(ctx, req); err != nil {
		c.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to refresh via the API", slog.Any("error", err), slog.String("reason", reason.String()))
	}
}

func (c *Client) Close() {
	if c == nil {
		return
	}

	if err := c.conn.Close(); err != nil {
		c.logger.LogAttrs(context.Background(), slog.LevelError, "Failed to close API client", slog.Any("error", err))
	}
}
