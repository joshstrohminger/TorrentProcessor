package work

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
	"golang.org/x/exp/maps"
)

// FileFormat abstracts the file format used for work entries.
type FileFormat interface {
	Ext() string
	Unmarshal(data []byte) (*torrent.Entry, error)
	Marshal(entry torrent.Entry) ([]byte, error)
}

// Work represents a file-based work queue.
//
// New files are added to represent a new entry in the queue. Files are processed oldest to newest based on modified time.
// Files are removed (deleted) once they're done being processed. When failing to process a file, it's left as is, but is
// added to an in-memory list so we know not to try processing it again. In theory, failures will be rare, so the list of
// ignored files shouldn't grow very much. To ensure it doesn't become a memory leak, it needs to be periodically re-checked
// to see if the files no longer exist and can be removed from the list.
type Work struct {
	dir        string
	logger     *slog.Logger
	ignored    map[string]struct{}
	fileFormat FileFormat
}

func New(path string, logger *slog.Logger) (*Work, error) {
	if info, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("invalid path '%s': %w", path, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: '%s'", path)
	} else {
		return &Work{
			dir:        path,
			ignored:    make(map[string]struct{}),
			logger:     logger,
			fileFormat: JsonFormat{},
		}, nil
	}
}

func (w *Work) getFilepath(entry torrent.Entry) string {
	return filepath.Join(w.dir, entry.Hash+w.fileFormat.Ext())
}

// Next finds the oldest unprocessed entry, or nil if nothing is found. Can return ErrParse or ErrIgnored based on whether ignore is set.
func (w *Work) Next(ignore bool) (*torrent.Entry, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read dir: %w", err)
	}

	// find the oldest unprocessed file in the directory
	var file fs.FileInfo
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == w.fileFormat.Ext() {
			if info, err := entry.Info(); err != nil {
				return nil, fmt.Errorf("failed to get entry info: %w", err)
			} else if _, exists := w.ignored[entry.Name()]; !exists && (file == nil || info.ModTime().Before(file.ModTime())) {
				file = info
			}
		}
	}
	if file == nil {
		return nil, nil
	}

	// read it
	path := filepath.Join(w.dir, file.Name())
	data, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("failed to read '%s': %w", path, err)
		if ignore {
			return nil, w.ignore(file.Name(), err)
		}
		return nil, ErrParse{err, path}
	}

	// parse it
	entry, err := w.fileFormat.Unmarshal(data)
	if err != nil {
		err = fmt.Errorf("failed to parse '%s': %w", path, err)
		if ignore {
			return nil, w.ignore(file.Name(), err)
		}
		return nil, ErrParse{err, path}
	}

	// check for oddities
	if expectedPath := w.getFilepath(*entry); expectedPath != path {
		// mark a failed file as processed so we ignore it since the contents have issues
		return nil, w.ignore(file.Name(), fmt.Errorf("next file '%s' should actually be named '%s' based on the contents", path, expectedPath))
	}

	return entry, nil
}

func (w *Work) CleanupIgnored() {
	names := maps.Keys(w.ignored)
	for _, name := range names {
		filename := filepath.Join(w.dir, name)
		if _, err := os.Stat(filename); errors.Is(err, fs.ErrNotExist) {
			delete(w.ignored, name)
			w.logger.Info("Cleaned up ignored entry", slog.String("file", name))
		}
	}
}

// ignore the file (mark it as processed) and log the provided error
func (w *Work) ignore(name string, err error) error {
	if err != nil {
		log := w.logger.Error
		if errors.Is(err, torrent.ErrManualProcessing) {
			log = w.logger.Warn
		}
		log("Ignoring file", slog.String("file", name), slog.Any("reason", err))
	}

	w.ignored[name] = struct{}{}

	return ErrIgnored{
		Err:      err,
		FileName: name,
	}
}

// Ignore the entry (mark it as procesed) and log the provided error
func (w *Work) Ignore(entry torrent.Entry, err error) {
	name := filepath.Base(w.getFilepath(entry))
	_ = w.ignore(name, err)
}

// Remove the entry
func (w *Work) Remove(entry torrent.Entry) error {
	path := w.getFilepath(entry)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove file '%s': %w", path, err)
	}
	return nil
}

// Add a new entry file
func (w *Work) Add(entry torrent.Entry) (err error) {
	data, err := w.fileFormat.Marshal(entry)
	if err != nil {
		return err
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

// Ignored returns the list of currently ignored files
func (w *Work) Ignored() []string {
	ignored := maps.Keys(w.ignored)
	slices.Sort(ignored)
	return ignored
}
