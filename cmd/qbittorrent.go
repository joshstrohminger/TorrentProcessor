package cmd

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/anacrolix/torrent/bencode"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var qBittorrentCmd = &cobra.Command{
	Use:               "qbittorrent",
	Short:             "Interact with qBittorrent",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
}

var qBittorrentInvocationCmd = &cobra.Command{
	Use:     "command",
	Aliases: []string{"add", "cmd"},
	Short:   "Print command for qBittorrent to add torrents",
	Long:    "Print the command for qBittorrent to add an entry when finishing a torrent.",

	RunE: func(cmd *cobra.Command, args []string) error {
		// See supported parameters in qBittorrent > Options > Downloads > Run External Program
		return printInvocation(map[string]string{
			"name":         "%N",
			"category":     "%L",
			"content-path": "%F",
			"save-path":    "%D",
			"num-files":    "%C",
			"size":         "%Z",
			"tracker":      "%T",
			"hash":         "%I",
		})
	},
}

var qBittorrentMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate torrents and qBittorrent data",
	Long:  "Migrate torrents, content, and qBittorrent data to a new instance.",

	RunE: func(cmd *cobra.Command, args []string) error {
		srcDir, err := cmd.Flags().GetString("source")
		if err != nil {
			return err
		}

		if srcDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get user home directory: %w", err)
			}

			switch runtime.GOOS {
			case "darwin":
				srcDir = filepath.Join(home, "Library/Application Support")

			case "linux":
				srcDir = filepath.Join(home, ".local/share")

			case "windows":
				srcDir = filepath.Join(home, `AppData\Local`)
			}

			srcDir = filepath.Join(srcDir, "qBittorrent", "BT_backup")
		}

		if strings.EqualFold("qBittorrent", filepath.Base(srcDir)) {
			srcDir = filepath.Join(srcDir, "BT_backup")
		} else if !strings.EqualFold("BT_backup", filepath.Base(srcDir)) {
			return fmt.Errorf("invalid directory: should point to qBittorrent or BT_backup")
		}

		if info, err := os.Stat(srcDir); err != nil {
			return fmt.Errorf("invalid directory %s: %w", srcDir, err)
		} else if !info.IsDir() {
			return fmt.Errorf("invalid directory %s: not a directory", srcDir)
		}

		srcDir, err = filepath.Abs(filepath.Clean(srcDir))
		if err != nil {
			return fmt.Errorf("failed to make source directory absolute: %w", err)
		}

		dstDir, err := cmd.Flags().GetString("destination")
		if err != nil {
			return err
		}
		if _, err := os.Stat(dstDir); err != nil {
			return fmt.Errorf("invalid destination directory %s: %w", dstDir, err)
		}

		dstDir, err = filepath.Abs(filepath.Clean(dstDir))
		if err != nil {
			return fmt.Errorf("failed to make destination directory absolute: %w", err)
		}

		if strings.EqualFold(srcDir, dstDir) {
			return fmt.Errorf("source and destination directories cannot be the same")
		}

		dataDir, err := cmd.Flags().GetString("data")
		if err != nil {
			return err
		}
		qbDataDir := filepath.ToSlash(dataDir)

		contentDstDir, err := cmd.Flags().GetString("content")
		if err != nil {
			return err
		}
		if _, err := os.Stat(contentDstDir); err != nil {
			return fmt.Errorf("content destination directory doesn't exist: %s", contentDstDir)
		}

		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}

		return convertFiles(qbConfig{
			srcDir:        srcDir,
			dstDir:        dstDir,
			dataDir:       dataDir,
			qbDataDir:     qbDataDir,
			contentDstDir: contentDstDir,
			dryRun:        dryRun,
		})
	},
}

type qBittorrentPair struct {
	Hash            string
	TorrentPath     string
	FastResumePath  string
	PreviousSaveDir string
	ContentSize     uint64
}

type qbConfig struct {
	srcDir        string
	dstDir        string
	dataDir       string
	qbDataDir     string
	contentDstDir string
	dryRun        bool
}

func convertFiles(cfg qbConfig) error {
	// find matching .torrent and .fastresume files
	pairs := make(map[string]*qBittorrentPair)
	entries, err := os.ReadDir(cfg.srcDir)
	if err != nil {
		return fmt.Errorf("failed to read from directory %s: %w", cfg.srcDir, err)
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
			pair.FastResumePath = filepath.Join(cfg.srcDir, entry.Name())
		} else if strings.EqualFold(".torrent", ext) {
			pair.TorrentPath = filepath.Join(cfg.srcDir, entry.Name())
		} else {
			continue
		}

		pairs[hash] = pair
	}

	for _, pair := range pairs {
		if pair.FastResumePath == "" || pair.TorrentPath == "" {
			return fmt.Errorf("incomplete pair of files: %v", pair)
		}
	}

	var copied uint64

	for _, pair := range pairs {
		if err := processPair(pair, cfg); err != nil {
			return fmt.Errorf("failed to process pair for hash %s: %w", pair.Hash, err)
		}
		copied += pair.ContentSize
	}

	if cfg.contentDstDir != "" {
		fmt.Printf("Copied %s of content\n", humanize.Bytes(copied))
	}

	return nil
}

func processPair(pair *qBittorrentPair, cfg qbConfig) error {
	fmt.Println("Processing hash", pair.Hash)

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

func processTorrent(pair *qBittorrentPair, cfg qbConfig) error {
	src := pair.TorrentPath
	dst := filepath.Join(cfg.dstDir, filepath.Base(pair.TorrentPath))
	if err := copyFileOrDir(src, dst, cfg.dryRun, &pair.ContentSize); err != nil {
		return fmt.Errorf("failed to copy torrent from %s to %s: %w", src, dst, err)
	}

	if cfg.contentDstDir == "" {
		return nil
	}

	const infoKey = "info"
	const nameKey = "name"

	data, err := os.ReadFile(pair.TorrentPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", pair.TorrentPath, err)
	}

	torrent := make(map[string]any)
	if err := bencode.Unmarshal(data, &torrent); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", pair.TorrentPath, err)
	}

	info, err := getMapValue[map[string]any](torrent, infoKey)
	if err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}

	name, err := getMapValue[string](info, nameKey)
	if err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}

	src = filepath.Join(pair.PreviousSaveDir, name)
	dst = filepath.Join(cfg.contentDstDir, name)
	if err := copyFileOrDir(src, dst, cfg.dryRun, &pair.ContentSize); err != nil {
		return fmt.Errorf("failed to copy data from %s to %s: %w", src, dst, err)
	}

	return nil
}

func processFastResume(pair *qBittorrentPair, cfg qbConfig) error {
	const dataDirKey = "save_path"
	const qbDataDirKey = "qBt-savePath"

	data, err := os.ReadFile(pair.FastResumePath)
	if err != nil {
		return fmt.Errorf("failed to read: %w", err)
	}

	fastResume := make(map[string]any)
	if err := bencode.Unmarshal(data, &fastResume); err != nil {
		return fmt.Errorf("failed to unmarshal: %w", err)
	}

	if pair.PreviousSaveDir, err = getMapValue[string](fastResume, dataDirKey); err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}
	fastResume[dataDirKey] = cfg.dataDir

	if _, err = getMapValue[string](fastResume, qbDataDirKey); err != nil {
		return fmt.Errorf("failed to read value: %w", err)
	}
	fastResume[qbDataDirKey] = cfg.qbDataDir

	data, err = bencode.Marshal(fastResume)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	dstPath := filepath.Join(cfg.dstDir, filepath.Base(pair.FastResumePath))

	fmt.Printf("Writing to %s\n", dstPath)
	if !cfg.dryRun {
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstPath, err)
		}
	}

	return nil
}

func getMapValue[T any](m map[string]any, key string) (value T, err error) {
	rawValue, ok := m[key]
	if !ok {
		err = fmt.Errorf("missing key: %s", key)
		return
	}

	value, ok = rawValue.(T)
	if !ok {
		err = fmt.Errorf("value of key %s type should be %T: %T", key, value, rawValue)
	}

	return
}

func printInvocation(mapping map[string]string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	if inTemp, err := isFileBelowDir(exe, os.TempDir()); inTemp {
		// assume we're been run using "go run" and the working directory contains the source
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory")
		}
		exe = "go run " + wd
	} else if err != nil {
		return fmt.Errorf("failed to check if running from the temp dir: %w", err)
	}

	if err := validateFlagMapping(mapping); err != nil {
		return err
	}

	invocation := make([]string, 0, len(mapping)*2+2)
	invocation = append(invocation, exe, addCmd.Name())

	keys := slices.Collect(maps.Keys(mapping))
	slices.Sort(keys)
	for _, key := range keys {
		invocation = append(invocation, "--"+key, "\""+mapping[key]+"\"")
	}

	fmt.Println(strings.Join(invocation, " "))
	return nil
}

// validateFlagMapping returns an error if a mapping doesn't exist for a flag or is unused.
func validateFlagMapping(mapping map[string]string) (err error) {
	var missingMappings []string
	unusedMappings := maps.Clone(mapping)

	addCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if requiredAnnotation, found := flag.Annotations[cobra.BashCompOneRequiredFlag]; !found || requiredAnnotation[0] != "true" {
			// skip flags that are not required
			return
		}

		if _, ok := mapping[flag.Name]; !ok {
			missingMappings = append(missingMappings, flag.Name)
		} else {
			delete(unusedMappings, flag.Name)
		}
	})

	if len(missingMappings) > 0 {
		slices.Sort(missingMappings)
		err = fmt.Errorf("missing mapping for flags: %s", missingMappings)
	}

	if len(unusedMappings) > 0 {
		keys := slices.Collect(maps.Keys(unusedMappings))
		slices.Sort(keys)
		err = errors.Join(err, fmt.Errorf("unused mappings: %s", keys))
	}

	return
}

func init() {
	rootCmd.AddCommand(qBittorrentCmd)
	qBittorrentCmd.AddCommand(qBittorrentInvocationCmd, qBittorrentMigrateCmd)

	qBittorrentMigrateCmd.Flags().String("source", "", "Directory to search for qBittorrent files to convert")
	qBittorrentMigrateCmd.Flags().String("destination", "", "Destination directory to migrate the converted qBittorrent files to")
	qBittorrentMigrateCmd.MarkFlagRequired("destination")
	qBittorrentMigrateCmd.Flags().String("data", "", "New save path where the torrent data will be stored")
	qBittorrentMigrateCmd.MarkFlagRequired("data")
	qBittorrentMigrateCmd.Flags().String("content", "", "Optional directory to copy content files to")
	qBittorrentMigrateCmd.Flags().Bool("dry-run", false, "Don't convert or copy any files")
}
