package qbittorrent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/anacrolix/torrent/bencode"
	"github.com/dustin/go-humanize"
	"github.com/joshstrohminger/TorrentProcessor/internal/util"
	"github.com/joshstrohminger/TorrentProcessor/internal/util/term"
)

type PruneAnswer int

const (
	PrunePrompt PruneAnswer = iota
	PruneYes
	PruneNo
)

func Prune(dir string, answer PruneAnswer) error {
	pairs, err := findPairs(dir)
	if err != nil {
		return fmt.Errorf("failed to find qBittorrent file pairs: %w", err)
	}

	content := make(map[string]*qBittorrentPair)
	contentRoots := make(map[string]struct{})
	var candidates []string
	var size uint64

	// extract content paths
	for _, pair := range pairs {
		if err := readPair(pair); err != nil {
			return fmt.Errorf("failed to read pair with hash %s: %w", pair.Hash, err)
		}
		content[pair.PreviousContentPath] = pair
		contentRoots[filepath.Dir(pair.PreviousContentPath)] = struct{}{}
	}

	fmt.Printf("Walking %d content roots from %d torrents\n", len(contentRoots), len(content))

	for root := range contentRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			return fmt.Errorf("failed to read content root %s: %w", root, err)
		}

		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			if _, ok := content[path]; !ok {
				if strings.HasPrefix(entry.Name(), "._") {
					// this must be a meta-data file used by macos, check if it has been orphaned and can be included as a candidate
					mainPath := filepath.Join(root, strings.TrimPrefix(entry.Name(), "._"))
					if _, err := os.Stat(mainPath); err == nil {
						// don't include those that are paired with a main file, we'll only prune them if the main file is pruned
						continue
					}
				}

				candidates = append(candidates, path)

				info, err := entry.Info()
				if err != nil {
					return fmt.Errorf("failed to get info for %s: %w", path, err)
				}

				if entry.IsDir() {
					// walk the dir, totally the size
					if err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
						if !d.IsDir() {
							info, err := d.Info()
							if err != nil {
								return fmt.Errorf("failed to get info for %s: %w", path, err)
							}
							size += uint64(info.Size())
						}
						return nil
					}); err != nil {
						return fmt.Errorf("failed to walk dir %s: %w", path, err)
					}
				} else {
					size += uint64(info.Size())
				}
			}
		}
	}

	sizeString := humanize.Bytes(size)
	fmt.Printf("Found %d pruning candidate(s) totalling %s\n", len(candidates), sizeString)
	slices.Sort(candidates)
	for _, path := range candidates {
		fmt.Println(path)
	}

	if len(candidates) == 0 {
		return nil
	}

	if answer == PrunePrompt {
		if !term.IsInteractive() {
			return fmt.Errorf("can't prompt for pruning action in a non-interactive session")
		}

		yes, err := term.PromptYesNo(fmt.Sprintf("Prune %s?", sizeString))
		if err != nil {
			return fmt.Errorf("failed to prompt for pruning action: %w", err)
		}

		if yes {
			answer = PruneYes
		} else {
			answer = PruneNo
		}
	}

	if answer == PruneNo {
		fmt.Println("Nothing pruned")
		return nil
	}

	fmt.Println("Pruning...")
	for _, path := range candidates {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to prune %s: %w", path, err)
		}

		name := filepath.Base(path)
		if !strings.HasPrefix(name, "._") {
			// check if a metadata file exists
			metaPath := filepath.Join(filepath.Dir(path), "._"+name)
			if _, err := os.Stat(metaPath); err == nil {
				// remove the "._" metadata file too
				if err := os.Remove(metaPath); err != nil {
					return fmt.Errorf("failed to remove metadata file %s: %w", metaPath, err)
				}
			}
		}
	}
	fmt.Println("Pruned", sizeString)

	return nil
}

func readPair(pair *qBittorrentPair) error {
	if _, err := readFastResume(pair); err != nil {
		return fmt.Errorf("fastresume: %w", err)
	}

	if err := readTorrent(pair); err != nil {
		return fmt.Errorf("torrent: %w", err)
	}

	return nil
}

// findPairs returns matching .torrent and .fastresume file pairs
func findPairs(dir string) (map[string]*qBittorrentPair, error) {
	pairs := make(map[string]*qBittorrentPair)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read from directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		hash := strings.TrimSuffix(entry.Name(), ext)
		pair, ok := pairs[hash]
		if !ok {
			pair = &qBittorrentPair{Hash: hash}
		}

		if strings.EqualFold(".fastresume", ext) {
			pair.FastResumePath = filepath.Join(dir, entry.Name())
		} else if strings.EqualFold(".torrent", ext) {
			pair.TorrentPath = filepath.Join(dir, entry.Name())
		} else {
			continue
		}

		pairs[hash] = pair
	}

	for _, pair := range pairs {
		if pair.FastResumePath == "" || pair.TorrentPath == "" {
			return nil, fmt.Errorf("incomplete pair of files: %v", pair)
		}
	}

	return pairs, nil
}

func readFastResume(pair *qBittorrentPair) (map[string]any, error) {
	data, err := os.ReadFile(pair.FastResumePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}

	fastResume := make(map[string]any)
	if err := bencode.Unmarshal(data, &fastResume); err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	if pair.PreviousSaveDir, err = util.GetMapValue[string](fastResume, fastResumeDataDirKey); err != nil {
		return nil, fmt.Errorf("failed to read value: %w", err)
	}

	return fastResume, nil
}

func readTorrent(pair *qBittorrentPair) error {
	data, err := os.ReadFile(pair.TorrentPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", pair.TorrentPath, err)
	}

	torrent := make(map[string]any)
	if err := bencode.Unmarshal(data, &torrent); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", pair.TorrentPath, err)
	}

	info, err := util.GetMapValue[map[string]any](torrent, torrentInfoKey)
	if err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}

	name, err := util.GetMapValue[string](info, torrentNameKey)
	if err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}

	sanitizedName := invalidPathCharsRegex.ReplaceAllString(name, "_")
	if sanitizedName != name {
		fmt.Printf("Sanitized name from %s to %s\n", name, sanitizedName)
	}

	pair.PreviousContentPath = filepath.Join(pair.PreviousSaveDir, name)

	return nil
}
