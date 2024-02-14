package work

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

type Work struct {
	dir       string
	logger    *slog.Logger
	processed map[string]struct{}
}

func New(path string, logger *slog.Logger) (*Work, error) {
	if info, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("invalid path '%s': %w", path, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: '%s'", path)
	} else {
		return &Work{
			dir:       path,
			processed: make(map[string]struct{}),
			logger:    logger}, nil
	}
}

func (w *Work) getFilepath(entry torrent.Entry) string {
	return filepath.Join(w.dir, entry.Hash+".json")
}

type ErrParse struct {
	Err      error
	Filepath string
}

func (e ErrParse) Error() string {
	return fmt.Errorf("failed to parse JSON from '%s': %w", e.Filepath, e.Err).Error()
}

func (w *Work) Next(ignore bool) (*torrent.Entry, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir: %w", err)
	}

	var file fs.FileInfo
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			if info, err := entry.Info(); err != nil {
				return nil, fmt.Errorf("failed to get entry info: %w", err)
			} else if _, exists := w.processed[entry.Name()]; !exists && (file == nil || info.ModTime().Before(file.ModTime())) {
				file = info
			}
		}
	}
	if file == nil {
		return nil, nil
	}

	entry := new(torrent.Entry)
	path := filepath.Join(w.dir, file.Name())
	if data, err := os.ReadFile(path); err != nil {
		err = fmt.Errorf("failed to read '%s': %w", path, err)
		if ignore {
			w.ignore(file.Name(), err)
			return nil, nil
		}
		return nil, ErrParse{err, path}
	} else if err = json.Unmarshal(data, entry); err != nil {
		err = fmt.Errorf("next file failed to parse '%s': %w", path, err)
		if ignore {
			w.ignore(file.Name(), err)
			return nil, nil
		}
		return nil, ErrParse{err, path}
	} else if expectedPath := w.getFilepath(*entry); expectedPath != path {
		// mark a failed file as processed so we ignore it since the contents have issues
		w.ignore(file.Name(), fmt.Errorf("next file '%s' should actually be named '%s' based on the contents", path, expectedPath))
		return nil, nil
	}
	return entry, nil
}

func (w *Work) ignore(name string, err error) {

	if err != nil {
		log := w.logger.Error
		if errors.Is(err, torrent.ErrManualProcessing) {
			log = w.logger.Warn
		}
		log("Ignoring file", slog.String("file", name), slog.Any("reason", err))
	}
	w.processed[name] = struct{}{}
}

func (w *Work) Ignore(entry torrent.Entry, err error) {
	name := filepath.Base(w.getFilepath(entry))
	w.ignore(name, err)
}

func (w *Work) Remove(entry torrent.Entry) error {
	path := w.getFilepath(entry)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to removed file '%s': %w", path, err)
	}
	return nil
}

func (w *Work) Add(entry torrent.Entry) (err error) {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	path := w.getFilepath(entry)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %w", path, err)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			err = errors.Join(err, fmt.Errorf("failed to close file '%s': %w", path, errClose))
		}
	}()

	if _, err = file.Write(data); err != nil {
		return fmt.Errorf("failed to write file '%s': %w", path, err)
	}

	w.logger.Info("Added entry", slog.String("file", path), slog.Any("entry", entry))
	return nil
}
