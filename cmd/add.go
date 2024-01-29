package cmd

import (
	"fmt"
	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
	"github.com/joshstrohminger/TorrentProcessor/internal/work"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"strings"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a torrent to be processed",
	Long:  "Add a completed torrent to the work to be processed.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if workPath, err := getWorkPath(cmd); err != nil {
			return err
		} else if name, err := cmd.Flags().GetString("name"); err != nil {
			return err
		} else if category, err := getEnumFlag(cmd, "category", torrent.AllCategories); err != nil {
			return err
		} else if contentPath, err := cmd.Flags().GetString("content-path"); err != nil {
			return err
		} else if savePath, err := cmd.Flags().GetString("save-path"); err != nil {
			return err
		} else if numFiles, err := cmd.Flags().GetInt("num-files"); err != nil {
			return err
		} else if size, err := cmd.Flags().GetInt("size"); err != nil {
			return err
		} else if tracker, err := cmd.Flags().GetString("tracker"); err != nil {
			return err
		} else if hash, err := cmd.Flags().GetString("hash"); err != nil {
			return err
		} else if outputPath, err := cmd.Flags().GetString("output-path"); err != nil {
			return err
		} else if work, err := work.New(workPath); err != nil {
			return fmt.Errorf("failed to create work list: %w", err)
		} else {
			entry := torrent.Entry{
				OutputPath:    outputPath,
				Name:          name,
				Category:      category,
				ContentPath:   contentPath,
				NumberOfFiles: numFiles,
				Size:          size,
				Tracker:       tracker,
				Hash:          hash,
				SavePath:      savePath,
			}
			if err := work.Add(entry); err != nil {
				return fmt.Errorf("failed to add entry %#v: %w", entry, err)
			}
		}
		return nil
	},
}

func getEnumFlag[T fmt.Stringer](cmd *cobra.Command, name string, possible []T) (enum T, err error) {
	var value string
	if value, err = cmd.Flags().GetString(name); err != nil {
		return
	}

	for _, e := range possible {
		if strings.EqualFold(e.String(), value) {
			enum = e
			return
		}
	}

	err = fmt.Errorf("flag %s value '%s' is invalid", name, value)
	return
}

func init() {
	categoryNames := make([]string, len(torrent.AllCategories))
	for i, category := range torrent.AllCategories {
		categoryNames[i] = category.String()
	}

	addCmd.Flags().String("name", "", "Torrent name")
	addCmd.Flags().String("category", "", "Category of the torrent: "+strings.Join(categoryNames, ", "))
	addCmd.Flags().String("content-path", "", "Path to the content, same as root path for multi-file torrents")
	addCmd.Flags().String("save-path", "", "Path to the saved torrent directory")
	addCmd.Flags().Int("num-files", 0, "Number of files to process")
	addCmd.Flags().Int("size", 0, "Torrent size in bytes")
	addCmd.Flags().String("tracker", "", "Tracker used for this torrent")
	addCmd.Flags().String("hash", "", "Info hash")
	addCmd.Flags().String("output-path", "", "Root directory to output files")

	addCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if err := rootCmd.MarkFlagRequired(flag.Name); err != nil {
			panic(fmt.Errorf("failed to mark flag '%s' required: %v", flag.Name, err))
		}
	})

	rootCmd.AddCommand(addCmd)
}
