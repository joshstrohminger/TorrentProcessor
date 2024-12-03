package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joshstrohminger/TorrentProcessor/internal/client"
	"github.com/joshstrohminger/TorrentProcessor/internal/client/qbittorrent"
	"github.com/spf13/cobra"
)

var qBittorrentCmd = &cobra.Command{
	Use:               "qbittorrent",
	Short:             "Interact with qBittorrent",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
}

var qBittorrentMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate torrents and qBittorrent data",
	Long:  "Migrate torrents, content, and qBittorrent data to a new instance.",

	RunE: func(cmd *cobra.Command, args []string) error {
		srcDir, err := getSrcDirFlag(cmd)
		if err != nil {
			return err
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

		cmd.SilenceUsage = true

		return qbittorrent.Migrate(qbittorrent.Config{
			SrcDir:        srcDir,
			DstDir:        dstDir,
			DataDir:       dataDir,
			QbDataDir:     qbDataDir,
			ContentDstDir: contentDstDir,
			DryRun:        dryRun,
		})
	},
}

func getSrcDirFlag(cmd *cobra.Command) (string, error) {
	srcDir, err := cmd.Flags().GetString("source")
	if err != nil {
		return "", err
	}

	if srcDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
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
		return "", fmt.Errorf("invalid directory: should point to qBittorrent or BT_backup")
	}

	if info, err := os.Stat(srcDir); err != nil {
		return "", fmt.Errorf("invalid directory %s: %w", srcDir, err)
	} else if !info.IsDir() {
		return "", fmt.Errorf("invalid directory %s: not a directory", srcDir)
	}

	srcDir, err = filepath.Abs(filepath.Clean(srcDir))
	if err != nil {
		return "", fmt.Errorf("failed to make source directory absolute: %w", err)
	}

	return srcDir, nil
}

var qBittorrentInvocationCmd = &cobra.Command{
	Use:     "command",
	Aliases: []string{"add", "cmd"},
	Short:   "Print command for qBittorrent to add torrents",
	Long:    "Print the command for qBittorrent to add an entry when finishing a torrent.",

	RunE: func(cmd *cobra.Command, args []string) error {
		// See supported parameters in qBittorrent > Options > Downloads > Run External Program
		return client.PrintInvocation(addCmd, map[string]string{
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

var qBittorrentPruneCmd = &cobra.Command{
	Use:     "prune",
	Aliases: []string{"clean"},
	Short:   "Delete orphaned content",
	Long:    "Search all content paths based on the loaded torrents and prompt to delete all content that doesn't match one of them.",

	RunE: func(cmd *cobra.Command, args []string) error {
		srcDir, err := getSrcDirFlag(cmd)
		if err != nil {
			return err
		}

		answer := qbittorrent.PrunePrompt

		if yes, err := cmd.Flags().GetBool("yes"); err != nil {
			return err
		} else if yes {
			answer = qbittorrent.PruneYes
		}

		if no, err := cmd.Flags().GetBool("no"); err != nil {
			return err
		} else if no {
			answer = qbittorrent.PruneNo
		}

		return qbittorrent.Prune(srcDir, answer)
	},
}

func init() {
	rootCmd.AddCommand(qBittorrentCmd)
	qBittorrentCmd.AddCommand(qBittorrentInvocationCmd, qBittorrentMigrateCmd, qBittorrentPruneCmd)

	qBittorrentMigrateCmd.Flags().String("source", "", "Directory to search for qBittorrent files to convert")
	qBittorrentMigrateCmd.Flags().String("destination", "", "Destination directory to migrate the converted qBittorrent files to")
	qBittorrentMigrateCmd.MarkFlagRequired("destination")
	qBittorrentMigrateCmd.Flags().String("data", "", "New save path where the torrent data will be stored")
	qBittorrentMigrateCmd.MarkFlagRequired("data")
	qBittorrentMigrateCmd.Flags().String("content", "", "Optional directory to copy content files to")
	qBittorrentMigrateCmd.Flags().Bool("dry-run", false, "Don't convert or copy any files")

	qBittorrentPruneCmd.Flags().String("source", "", "Directory to search for qBittorrent files to use for pruning")
	qBittorrentPruneCmd.Flags().BoolP("yes", "y", false, "Automatic 'yes' answer to prompts")
	qBittorrentPruneCmd.Flags().BoolP("no", "n", false, "Automatic 'no' answer to prompts")
	qBittorrentPruneCmd.MarkFlagsMutuallyExclusive("yes", "no")
}
