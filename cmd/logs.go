package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"
)

const debugLogName = "tp.debug.log"

var logsCmd = &cobra.Command{
	Use:               "logs",
	Aliases:           []string{"log"},
	Short:             "Interact with logs",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		fmt.Println(cfg.LogPath)

		return nil
	},
}

var listLogsCmd = &cobra.Command{
	Use:               "list",
	Aliases:           []string{"ls"},
	Short:             "List log files",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		files, err := filepath.Glob(filepath.Join(cfg.LogPath, fmt.Sprintf(logNameFormat, "*")))
		if err != nil {
			return err
		}

		for _, file := range files {
			fmt.Println(file)
		}

		return nil
	},
}

var viewLogsCmd = &cobra.Command{
	Use:               "view",
	Aliases:           []string{"open", "code"},
	Short:             "View log files in VsCode",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		path, err := exec.LookPath("code")
		if err != nil {
			return fmt.Errorf("can't find VsCode app (code) in the path")
		}

		out, err := exec.Command(path, cfg.LogPath).CombinedOutput()
		output := string(out)
		if output != "" {
			fmt.Println(output)
		}
		return err
	},
}

const defaultSummaryLimit = 72 * time.Hour

var summarizeLogsCmd = &cobra.Command{
	Use:               "summarize",
	Aliases:           []string{"summary"},
	Short:             "Summarize the number of logs of each level",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		limit, err := cmd.Flags().GetDuration("limit")
		if err != nil {
			return err
		}
		if limit < 0 {
			limit *= -1
		}

		files, err := filepath.Glob(filepath.Join(cfg.LogPath, fmt.Sprintf(logNameFormat, "*")))
		if err != nil {
			return err
		}

		cutoff := time.Now().Add(-limit)
		totalCounts := make(map[slog.Level]int)
		var fileCount int

		for _, file := range files {
			if filepath.Base(file) == debugLogName {
				continue
			}

			counts, err := getLogFileSummary(file, cutoff)
			if err != nil {
				return fmt.Errorf("failed to summarize file %s: %w", file, err)
			}

			if len(counts) > 0 {
				fileCount++
				for level, count := range counts {
					totalCounts[level] += count
				}
			}
		}

		var total int
		var maxLevelLen int
		levels := make([]slog.Level, 0, len(totalCounts))

		for level, count := range totalCounts {
			maxLevelLen = max(maxLevelLen, len(level.String()))
			total += count
			levels = append(levels, level)
		}
		slices.Sort(levels)

		fmt.Printf("Found %d log entries since %s (%s limit) across %d file(s)\n", total, cutoff.Format(time.DateTime), limit, fileCount)
		for _, level := range levels {
			fmt.Printf("%*s%d\n", -maxLevelLen-1, level, totalCounts[level])
		}
		return nil
	},
}

func getLogFileSummary(path string, cutoff time.Time) (map[slog.Level]int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat: %w", err)
	}

	if info.ModTime().Before(cutoff) {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open: %w", err)
	}
	defer file.Close()

	entry := struct {
		Time  time.Time  `json:"time"`
		Level slog.Level `json:"level"`
	}{}

	counts := make(map[slog.Level]int)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("failed to parse entry '%s': %w", string(line), err)
		}

		if entry.Time.After(cutoff) {
			counts[entry.Level]++
		}
	}

	return counts, nil
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(listLogsCmd, viewLogsCmd, summarizeLogsCmd)

	summarizeLogsCmd.Flags().Duration("limit", defaultSummaryLimit, "How far into the past to look")
}
