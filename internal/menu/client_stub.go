//go:build !darwin

package menu

import (
	"context"
	"log/slog"

	"github.com/joshstrohminger/TorrentProcessor/internal/api"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
)

type Client struct {
}

func NewClient(cfg config.App, logger *slog.Logger) *Client {
	return new(Client)
}

func (c *Client) Refresh(ctx context.Context, reason api.RefreshRequest_Reason) {
	// noop
}

func (c *Client) Close() {
	// noop
}
