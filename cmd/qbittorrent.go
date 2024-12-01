package cmd

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/anacrolix/torrent/bencode"
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

var qBittorrentExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export torrent and output paths from qBittorrent",
	Long:  "Export torrent and output paths from qBittorrent for migrating from one system or location to another.",

	RunE: func(cmd *cobra.Command, args []string) error {
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}

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

		return convertFiles(srcDir, dstDir, dataDir, qbDataDir, force)
	},
}

func convertFiles(srcDir string, dstDir string, dataDir string, qbDataDir string, force bool) error {
	// find matching .torrent and .fastresume files
	type Pair struct {
		Hash           string
		TorrentPath    string
		FastResumePath string
	}

	pairs := make(map[string]Pair)
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read from directory %s: %w", srcDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		hash := strings.TrimSuffix(entry.Name(), ext)
		pair := pairs[hash]

		if strings.EqualFold(".fastresume", ext) {
			pair.FastResumePath = filepath.Join(srcDir, entry.Name())
		} else if strings.EqualFold(".torrent", ext) {
			pair.TorrentPath = filepath.Join(srcDir, entry.Name())
		} else {
			continue
		}

		pair.Hash = hash
		pairs[hash] = pair
	}

	for _, pair := range pairs {
		if pair.FastResumePath == "" || pair.TorrentPath == "" {
			return fmt.Errorf("incomplete pair of files: %v", pair)
		}
	}

	const dataDirKey = "save_path"
	const qbDataDirKey = "qBt-savePath"

	for _, pair := range pairs {
		data, err := os.ReadFile(pair.FastResumePath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", pair.FastResumePath, err)
		}

		fastResume := make(map[string]any)
		if err := bencode.Unmarshal(data, &fastResume); err != nil {
			return fmt.Errorf("failed to unmarshal %s: %w", pair.FastResumePath, err)
		}

		outputDirValue, ok := fastResume[dataDirKey]
		if !ok {
			return fmt.Errorf("missing existing %s key in fastresume", dataDirKey)
		}
		outputDir, ok := outputDirValue.(string)
		if !ok {
			return fmt.Errorf("%s value is type %T instead of string in torrent: %s", dataDirKey, outputDirValue, pair.TorrentPath)
		}
		fastResume[dataDirKey] = dataDir

		if _, ok := fastResume[qbDataDirKey]; !ok {
			return fmt.Errorf("missing existing %s key in fastresume", qbDataDirKey)
		}
		fastResume[qbDataDirKey] = qbDataDir

		data, err = bencode.Marshal(fastResume)
		if err != nil {
			return fmt.Errorf("failed to marshal %s: %w", filepath.Base(pair.FastResumePath), err)
		}

		dstPath := filepath.Join(dstDir, filepath.Base(pair.FastResumePath))
		if !force {
			if _, err := os.Stat(dstPath); err == nil {
				return fmt.Errorf("destination already exists: %s", dstPath)
			}
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstPath, err)
		}

		data, err = os.ReadFile(pair.TorrentPath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", pair.TorrentPath, err)
		}

		torrent := make(map[string]any)
		if err := bencode.Unmarshal(data, &torrent); err != nil {
			return fmt.Errorf("failed to unmarshal %s: %w", pair.TorrentPath, err)
		}

		if infoValue, ok := torrent["info"]; !ok {
			return fmt.Errorf("can't find info key in torrent: %s", pair.TorrentPath)
		} else if info, ok := infoValue.(map[string]any); !ok {
			return fmt.Errorf("info value is type %T instead of a dictionary in torrent: %s", infoValue, pair.TorrentPath)
		} else if nameValue, ok := info["name"]; !ok {
			return fmt.Errorf("can't find name key in torrent info: %s", pair.TorrentPath)
		} else if name, ok := nameValue.(string); !ok {
			return fmt.Errorf("name value is type %T instead of a string in torrent: %s", nameValue, pair.TorrentPath)
		} else {
			outputPath := filepath.Join(outputDir, name)
			_, err := os.Stat(outputPath)
			fmt.Printf("%s exists? %t\n", outputPath, err == nil)
		}

		break
	}

	return nil
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
	qBittorrentCmd.AddCommand(qBittorrentInvocationCmd, qBittorrentExportCmd)

	qBittorrentExportCmd.Flags().String("source", "", "Directory to search for qBittorrent files to convert")
	qBittorrentExportCmd.Flags().String("destination", "", "Destination directory to export the converted files")
	qBittorrentExportCmd.MarkFlagRequired("destination")
	qBittorrentExportCmd.Flags().String("data", "", "New save path where the torrent data will be stored")
	qBittorrentExportCmd.MarkFlagRequired("data")
	qBittorrentExportCmd.Flags().Bool("force", false, "Overwrite existing exported files")
}
