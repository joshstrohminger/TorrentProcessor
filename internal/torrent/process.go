package torrent

import (
	"context"
	"errors"
	"fmt"
	"github.com/joshstrohminger/TorrentProcessor/internal/config"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
)

var subtitleExts = []string{".srt", ".smi", ".ssa", ".ass", ".vtt"}

type Processor struct {
	cfg    config.Process
	logger *slog.Logger
}

func NewProcessor(cfg config.Process, logger *slog.Logger) *Processor {
	return &Processor{cfg, logger}
}

func (p *Processor) Process(ctx context.Context, entry Entry) error {
	p.logger.LogAttrs(ctx, slog.LevelInfo, "Processing", slog.Any("config", p.cfg), slog.Any("entry", entry))

	switch entry.Category {
	case MovieSingle:
		return p.copyMovieSingle(ctx, entry)
	case TvSingle:
	case TvSeason:
	case Ignore:
	default:
		return fmt.Errorf("unhandled category %s", entry.Category)
	}
}

func (p *Processor) copyMovieSingle(ctx context.Context, entry Entry) error {
	if entry.NumberOfFiles <= 0 {
		return errors.New("no files")
	}

	subtitles := &FileFilter{
		Filter: func(s string) bool {
			return slices.Contains(subtitleExts, path.Ext(s))
		},
	}
	videos := &FileFilter{
		Filter: func(s string) bool {
			return !slices.Contains(subtitleExts, path.Ext(s))
		},
	}
	if err := filterFiles(entry.ContentPath, subtitles, videos); err != nil {
		return fmt.Errorf("failed to filter files: %w", err)
	}
	if len(videos.Files) == 0 {
		return fmt.Errorf("no video files found in %s", entry.ContentPath)
	}
	if len(videos.Files) > 1 {
		return fmt.Errorf("found %d video files but can only handle one: %v", len(videos.Files), videos.Files)
	}

	destination := path.Clean(path.Join(p.cfg.MovieOutputPath, entry.Name+path.Ext(videos.Files[0])))
	if _, err := os.Stat(destination); err != nil {
		return fmt.Errorf("video destination already exists: %s", destination)
	}

	if err := p.copyFile(videos.Files[0], destination); err != nil {
		return fmt.Errorf("failed to copy video: %w", err)
	}

	if len(subtitles.Files) > 0 {
		// only process the first subtitle of each extension, assuming english
		processed := make(map[string]struct{})

		for _, file := range subtitles.Files {
			ext := path.Ext(file)
			if _, exists := processed[ext]; exists {
				p.logger.LogAttrs(ctx, slog.LevelWarn, "Subtitle skipped because we've already processed one for this extension", slog.String("subtitle", file))
			}
			processed[ext] = struct{}{}
			destination = path.Clean(path.Join(p.cfg.MovieOutputPath, fmt.Sprintf("%s.en%s", entry.Name, ext)))

			if err := p.copyFile(file, destination); err != nil {
				return fmt.Errorf("failed to copy subtitle: %w", err)
			}
		}
	}

	return nil
}

func (p *Processor) copyFile(src string, dst string) error {
	p.logger.Info("Copying from %s to %s", src, dst)

	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination already exists: %s", dst)
	}

	if !p.cfg.DryRun {
		out, err := os.Create(dst)
		if err != nil {
			return fmt.Errorf("failed to create destinaion file %s: %w", dst, err)
		}

		in, err := os.Open(src)
		if err != nil {
			return fmt.Errorf("failed to open source file %s: %w", src, err)
		}

		if _, err = io.Copy(out, in); err != nil {
			return fmt.Errorf("failed to copy file from %s to %s: %w", src, dst, err)
		}
	}

	return nil
}

type FileFilter struct {
	Files  []string
	Filter func(string) bool
}

func filterFiles(dir string, filters ...*FileFilter) error {
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			for _, filter := range filters {
				if filter.Filter(d.Name()) {
					filter.Files = append(filter.Files, d.Name())
				}
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to walk dir %s: %w", dir, err)
	}
	return nil
}
