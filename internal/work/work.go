package work

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
	"io/fs"
	"os"
	"path"
)

type Work struct {
	dir       string
	processed map[string]struct{}
}

func New(path string) (*Work, error) {
	if info, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("invalid path '%s': %w", path, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: '%s'", path)
	} else {
		return &Work{dir: path, processed: make(map[string]struct{})}, nil
	}
}

func (w *Work) getFilepath(entry torrent.Entry) string {
	return path.Join(w.dir, entry.Hash+".json")
}

type ErrParse struct {
	err      error
	filepath string
}

func (e ErrParse) Error() string {
	return fmt.Errorf("failed to parse JSON from '%s': %w", e.filepath, e.err).Error()
}

func (w *Work) Next() (*torrent.Entry, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir: %w", err)
	}

	var file fs.FileInfo
	for _, entry := range entries {
		if !entry.IsDir() && path.Ext(entry.Name()) == ".json" {
			if info, err := entry.Info(); err != nil {
				return nil, fmt.Errorf("failed to get entry info: %w", err)
			} else if _, exists := w.processed[file.Name()]; !exists && (file == nil || info.ModTime().Before(file.ModTime())) {
				file = info
			}
		}
	}
	if file == nil {
		return nil, nil
	}

	entry := new(torrent.Entry)
	filepath := path.Join(w.dir, file.Name())
	if data, err := os.ReadFile(filepath); err != nil {
		return nil, fmt.Errorf("failed to read '%s': %w", filepath, err)
	} else if err = json.Unmarshal(data, entry); err != nil {
		return nil, ErrParse{err, filepath}
	} else if expectedPath := w.getFilepath(*entry); expectedPath != filepath {
		// mark a failed file as processed so we ignore it since the contents have issues
		w.processed[file.Name()] = struct{}{}
		return nil, fmt.Errorf("file '%s' should actually be named '%s' based on the contents", filepath, expectedPath)
	}
	return entry, nil
}

func (w *Work) Ignore(entry torrent.Entry) {
	filepath := w.getFilepath(entry)
	w.processed[path.Base(filepath)] = struct{}{}
}

func (w *Work) Remove(entry torrent.Entry) error {
	filepath := w.getFilepath(entry)
	if err := os.Remove(filepath); err != nil {
		return fmt.Errorf("failed to removed file '%s': %w", filepath, err)
	}
	return nil
}

func (w *Work) Add(entry torrent.Entry) (err error) {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json: %w", err)
	}

	filepath := w.getFilepath(entry)
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %w", filepath, err)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			err = errors.Join(err, fmt.Errorf("failed to close file '%s': %w", filepath, errClose))
		}
	}()

	if _, err = file.Write(data); err != nil {
		return fmt.Errorf("failed to write file '%s': %w", filepath, err)
	}
	return nil
}
