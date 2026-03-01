package menu

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"

	"github.com/joshstrohminger/TorrentProcessor/internal/app"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"github.com/joshstrohminger/TorrentProcessor/internal/daemon"
)

//go:embed icon.icns
var icnsIcon []byte

//go:embed logo.pdf
var pdfIcon []byte

//go:embed app.plist.tmpl
var appPlistTemplate string

//go:embed startup.plist.tmpl
var startupPlistTemplate string

func getAppBundlePath() string {
	dir := config.GetUserAppConfigDir()
	return filepath.Join(dir, app.LongName+".app")
}

func Install(cfg config.App, logger *slog.Logger, version string, force bool, debugLogName string) error {
	const dirPerm = 0755
	const filePerm = 0644

	info, err := daemon.GetInfo()
	if err != nil {
		return err
	}

	bundle := getAppBundlePath()
	if _, err := os.Stat(bundle); err == nil {
		logger.LogAttrs(context.Background(), slog.LevelInfo, "App bundle already installed", slog.String("path", bundle))
		if !force {
			return nil
		}

		if err := Uninstall(cfg, logger); err != nil {
			return err
		}
	}

	contentsDir := filepath.Join(bundle, "Contents")
	if err := os.MkdirAll(contentsDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create app bundle dir: %w", err)
	}

	macOsDir := filepath.Join(contentsDir, "MacOS")
	if err := os.Mkdir(macOsDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create macos dir: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get exe: %w", err)
	}
	newLink := filepath.Join(macOsDir, filepath.Base(exe))
	if err := os.Symlink(exe, newLink); err != nil {
		return fmt.Errorf("failed to create app bundle symlink: %w", err)
	}

	plistPath := filepath.Join(contentsDir, "Info.plist")
	file, err := os.OpenFile(plistPath, os.O_WRONLY|os.O_CREATE, filePerm)
	if err != nil {
		return fmt.Errorf("failed to open app bundle plist: %w", err)
	}
	defer file.Close()

	t, err := template.New(filepath.Base(plistPath)).Parse(appPlistTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse app plist template: %w", err)
	}

	data := daemon.TemplateData{exe, info, cfg, version, app.LongName}
	data.Info.Label += ".menu"
	data.Info.DebugLogName = debugLogName
	if err := t.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute app plist template: %w", err)
	}

	resourceDir := filepath.Join(contentsDir, "Resources")
	if err := os.Mkdir(resourceDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create resources dir: %w", err)
	}

	iconPath := filepath.Join(resourceDir, "icon.icns")
	if err := os.WriteFile(iconPath, icnsIcon, filePerm); err != nil {
		return fmt.Errorf("failed to write icon: %w", err)
	}

	logoPath := filepath.Join(resourceDir, "logo.pdf")
	if err := os.WriteFile(logoPath, pdfIcon, filePerm); err != nil {
		return fmt.Errorf("failed to write logo: %w", err)
	}

	//TODO create startup daemon
	return nil
}

func Uninstall(cfg config.App, logger *slog.Logger) error {
	bundle := getAppBundlePath()
	logger.LogAttrs(context.Background(), slog.LevelDebug, "Removing app bundle", slog.String("path", bundle))
	if err := os.RemoveAll(bundle); err != nil {
		return fmt.Errorf("failed to remove app bundle: %w", err)
	}
	return nil
}
