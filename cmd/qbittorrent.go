package cmd

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var qbittorrentCmd = &cobra.Command{
	Use:   "qbittorrent",
	Short: "Print invocation for qBittorrent",
	Long:  "Print the command for qBittorrent to add an entry when finishing a torrent.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// override the root prerun
		return nil
	},
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
	addCmd.AddCommand(qbittorrentCmd)
}
