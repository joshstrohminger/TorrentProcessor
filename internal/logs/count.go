package logs

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/joshstrohminger/TorrentProcessor/internal/app"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
)

const NameFormat = app.ShortName + ".%s.log"
const debugLogName = app.ShortName + ".debug.log"

func CountLevels(cfg config.App, cutoff time.Time) (map[slog.Level]int, error) {
	totalCounts := make(map[slog.Level]int)
	var fileCount int

	files, err := filepath.Glob(filepath.Join(cfg.LogPath, fmt.Sprintf(NameFormat, "*")))
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if filepath.Base(file) == debugLogName {
			continue
		}

		counts, err := getLogFileSummary(file, cutoff)
		if err != nil {
			return nil, fmt.Errorf("failed to summarize file %s: %w", file, err)
		}

		if len(counts) > 0 {
			fileCount++
			for level, count := range counts {
				totalCounts[level] += count
			}
		}
	}

	return totalCounts, nil
}

func getLogFileSummary(path string, cutoff time.Time) (map[slog.Level]int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat: %w", err)
	}

	if info.ModTime().Before(cutoff) {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open: %w", err)
	}
	defer file.Close()

	entry := struct {
		Time  time.Time  `json:"time"`
		Level slog.Level `json:"level"`
	}{}

	counts := make(map[slog.Level]int)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("failed to parse entry '%s': %w", string(line), err)
		}

		if entry.Time.After(cutoff) {
			counts[entry.Level]++
		}
	}

	return counts, nil
}

func CountQueued(cfg config.App) (int, error) {
	entries, err := os.ReadDir(cfg.WorkPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read from work path %s: %w", cfg.WorkPath, err)
	}

	return len(entries), nil
}
