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
	"regexp"
	"slices"
	"strconv"
	"strings"
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

	if entry.NumberOfFiles <= 0 {
		return errors.New("no files")
	}

	if _, err := os.Stat(entry.ContentPath); err != nil {
		return fmt.Errorf("content path doesn't exist: %s", entry.ContentPath)
	}

	switch entry.Category {
	case MovieSingle:
		return p.copyMovieSingle(ctx, entry)
	case TvSingle:
		return p.copyTvSingle(ctx, entry)
	case TvSeason:
		return p.copyTvSeason(ctx, entry)
	case Ignore:
		return nil
	default:
		return fmt.Errorf("unhandled category %s", entry.Category)
	}
}

func (p *Processor) copyMovieSingle(ctx context.Context, entry Entry) error {
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

func (p *Processor) copyTvSeason(ctx context.Context, entry Entry) error {
	if entry.NumberOfFiles < 2 {
		return fmt.Errorf("need at least 2 files, entry has %d", entry.NumberOfFiles)
	}

	seasonInfo, err := parseTvSeason(entry.Name)
	if err != nil {
		return err
	}

	dir, err := p.mkTvDir(ctx, seasonInfo)
	if err != nil {
		return fmt.Errorf("failed to create TV dir: %w", err)
	}

	entries, err := os.ReadDir(entry.ContentPath)
	if err != nil {
		return fmt.Errorf("failed to read dir %s: %w", entry.ContentPath, err)
	}

	for _, srcEntry := range entries {
		if srcEntry.IsDir() || !strings.EqualFold(path.Ext(srcEntry.Name()), ".mkv") {
			continue
		}

		episodeInfo, err := parseTvEpisode(srcEntry.Name())
		if err != nil {
			return err
		}
		episodeInfo.Season = seasonInfo.Season

		if err := p.copyFile(path.Clean(path.Join(entry.ContentPath, srcEntry.Name())), path.Clean(path.Join(dir, episodeInfo.ToEpisodeName(path.Ext(srcEntry.Name()))))); err != nil {
			return fmt.Errorf("failed to copy: %w", err)
		}
	}

	return nil
}

func (p *Processor) copyTvSingle(ctx context.Context, entry Entry) error {
	if entry.NumberOfFiles > 1 {
		return errors.New("more than one file")
	}

	info, err := parseTvEpisode(entry.Name)
	if err != nil {
		return err
	}

	dir, err := p.mkTvDir(ctx, info)
	if err != nil {
		return fmt.Errorf("failed to create TV dir: %w", err)
	}

	if err := p.copyFile(entry.ContentPath, path.Clean(path.Join(dir, info.ToEpisodeName(path.Ext(entry.ContentPath))))); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return nil
}

func (p *Processor) mkTvDir(ctx context.Context, info TvInfo) (string, error) {
	// in case the case is wrong, check each directory to see if it matches
	entries, err := os.ReadDir(p.cfg.TvOutputPath)
	if err != nil {
		return "", fmt.Errorf("failed to read dir %s: %w", p.cfg.TvOutputPath, err)
	}

	var dir string
	for _, entry := range entries {
		if entry.IsDir() && strings.EqualFold(entry.Name(), info.Name) {
			dir = path.Clean(path.Join(p.cfg.TvOutputPath, entry.Name()))
			break
		}
	}

	if dir == "" {
		// doesn't exist, need to create it
		destination := path.Clean(path.Join(p.cfg.TvOutputPath, info.Name))
		p.logger.LogAttrs(ctx, slog.LevelInfo, "Creating TV show directory", slog.String("dir", destination))

		if !p.cfg.DryRun {
			if err := os.Mkdir(destination, 0666); err != nil {
				return "", fmt.Errorf("failed to create dir %s: %w", destination, err)
			}
		}
	}

	return dir, nil
}

type TvInfo struct {
	Name    string
	Season  int
	Episode int
}

func (t TvInfo) ToEpisodeName(ext string) string {
	return fmt.Sprintf("%s S%02dE%02d%s", t.Name, t.Season, t.Episode, ext)
}

func parseTvSeason(name string) (tv TvInfo, err error) {
	if reg, err := regexp.Compile(`(?i)^(.*?)S(\d+)`); err != nil {
		return tv, fmt.Errorf("failed to compile season regex: %w", err)
	} else if matches := reg.FindStringSubmatch(name); matches == nil {
		return tv, fmt.Errorf("failed to extract TV name/season from name %s", name)
	} else if season, err := strconv.Atoi(matches[2]); err != nil {
		return tv, fmt.Errorf("failed to convert season %s to number: %w", matches[2], err)
	} else {
		return TvInfo{
			Name:   matches[1],
			Season: season,
		}, nil
	}
}

func parseTvEpisode(name string) (tv TvInfo, err error) {
	if reg, err := regexp.Compile(`(?i)^(.*?)S(\d+)\.?E(\d+)`); err != nil {
		return tv, fmt.Errorf("failed to compile episode regex: %w", err)
	} else if matches := reg.FindStringSubmatch(name); matches == nil {
		return tv, fmt.Errorf("failed to extract TV name/season/episode from name %s", name)
	} else if season, err := strconv.Atoi(matches[2]); err != nil {
		return tv, fmt.Errorf("failed to convert season %s to number: %w", matches[2], err)
	} else if episode, err := strconv.Atoi(matches[3]); err != nil {
		return tv, fmt.Errorf("failed to convert episode %s to number: %w", matches[3], err)
	} else {
		return TvInfo{
			Name:    matches[1],
			Season:  season,
			Episode: episode,
		}, nil
	}
}
