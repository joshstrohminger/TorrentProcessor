package qbittorrent

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/anacrolix/torrent/bencode"
	"github.com/dustin/go-humanize"
	"github.com/joshstrohminger/TorrentProcessor/internal/util"
)

const torrentInfoKey = "info"
const torrentNameKey = "name"

const fastResumeDataDirKey = "save_path"
const fastResumeQbDataDirKey = "qBt-savePath"

type qBittorrentPair struct {
	Hash                string
	TorrentPath         string
	FastResumePath      string
	PreviousSaveDir     string
	PreviousContentPath string
	ContentSize         uint64
}

type Config struct {
	SrcDir        string
	DstDir        string
	DataDir       string
	QbDataDir     string
	ContentDstDir string
	DryRun        bool
}

func Migrate(cfg Config) error {
	pairs, err := findPairs(cfg.SrcDir)
	if err != nil {
		return fmt.Errorf("failed to find qBittorrent file pairs: %w", err)
	}

	var copied uint64

	var num int
	for _, pair := range pairs {
		num++
		fmt.Printf("Processing %d of %d: hash %s\n", num, len(pairs), pair.Hash)
		if err := processPair(pair, cfg); err != nil {
			return fmt.Errorf("failed to process pair for hash %s: %w", pair.Hash, err)
		}
		copied += pair.ContentSize
	}

	if cfg.ContentDstDir != "" {
		fmt.Printf("Copied %s of content\n", humanize.Bytes(copied))
	}

	return nil
}

func processPair(pair *qBittorrentPair, cfg Config) error {
	if err := processFastResume(pair, cfg); err != nil {
		return fmt.Errorf("fastresume: %w", err)
	}

	if err := processTorrent(pair, cfg); err != nil {
		return fmt.Errorf("torrent: %w", err)
	}

	return nil
}

func copyFileOrDir(src string, dst string, dryRun bool, copied *uint64) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source doesn't exist: %s", src)
	}

	if info.IsDir() {
		fmt.Printf("Processing directory %s\n", src)
		if _, err := os.Stat(dst); !dryRun && err != nil {
			if err := os.Mkdir(dst, info.Mode().Perm()); err != nil {
				return fmt.Errorf("failed to create destination directory %s: %w", dst, err)
			}
		}

		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("failed to read files from source directory %s: %w", src, err)
		}

		fmt.Printf("Copying %d entries in directory from %s to %s\n", len(entries), src, dst)
		for _, entry := range entries {
			childSrc := filepath.Join(src, entry.Name())
			childDst := filepath.Join(dst, entry.Name())
			if err := copyFileOrDir(childSrc, childDst, dryRun, copied); err != nil {
				return fmt.Errorf("failed to copy from %s to %s: %w", childSrc, childDst, err)
			}
		}

		return nil
	}

	*copied += uint64(info.Size())

	fmt.Printf("Copying %s from %s to %s\n", humanize.Bytes(uint64(info.Size())), src, dst)
	if _, err := os.Stat(dst); err == nil {
		fmt.Println("destination already exists")
		return nil
	}

	if !dryRun {
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

var invalidPathCharsRegex = regexp.MustCompile(`[<>:\\|?*"]`)

func processTorrent(pair *qBittorrentPair, cfg Config) error {
	src := pair.TorrentPath
	dst := filepath.Join(cfg.DstDir, filepath.Base(pair.TorrentPath))
	if err := copyFileOrDir(src, dst, cfg.DryRun, &pair.ContentSize); err != nil {
		return fmt.Errorf("failed to copy torrent from %s to %s: %w", src, dst, err)
	}

	if cfg.ContentDstDir == "" {
		return nil
	}

	if err := readTorrent(pair); err != nil {
		return err
	}

	src = pair.PreviousContentPath
	dst = filepath.Join(cfg.ContentDstDir, filepath.Base(src))
	if err := copyFileOrDir(src, dst, cfg.DryRun, &pair.ContentSize); err != nil {
		return fmt.Errorf("failed to copy content from %s to %s: %w", src, dst, err)
	}

	return nil
}

func processFastResume(pair *qBittorrentPair, cfg Config) error {
	fastResume, err := readFastResume(pair)
	if err != nil {
		return err
	}
	fastResume[fastResumeDataDirKey] = cfg.DataDir

	if _, err = util.GetMapValue[string](fastResume, fastResumeQbDataDirKey); err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}
	fastResume[fastResumeQbDataDirKey] = cfg.QbDataDir

	data, err := bencode.Marshal(fastResume)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	dstPath := filepath.Join(cfg.DstDir, filepath.Base(pair.FastResumePath))

	fmt.Printf("Writing to %s\n", dstPath)
	if !cfg.DryRun {
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstPath, err)
		}
	}

	return nil
}
