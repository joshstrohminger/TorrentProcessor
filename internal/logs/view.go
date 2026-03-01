package logs

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/joshstrohminger/TorrentProcessor/internal/config"
)

func View(cfg config.App) error {
	vsCodePath, err := exec.LookPath("code")
	if err != nil {
		// try to open finder
		switch runtime.GOOS {
		case "darwin":
			if err := exec.Command("open", cfg.LogPath).Run(); err != nil {
				return fmt.Errorf("failed to open finder: %w", err)
			}

		case "windows":
			if err := exec.Command("start", cfg.LogPath).Run(); err != nil {
				return fmt.Errorf("failed to open file explorer: %w", err)
			}

		default:
			return fmt.Errorf("can't call default directory handler for OS: %s", runtime.GOOS)
		}
	}

	if err := exec.Command(vsCodePath, cfg.LogPath).Run(); err != nil {
		return fmt.Errorf("failed to open VsCode: %w", err)
	}

	return nil
}
