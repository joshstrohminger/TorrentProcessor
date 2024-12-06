package cmd

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:               "config",
	Short:             "Interact with configuration",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	Args:              cobra.NoArgs,

	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := getAppConfig(cmd); err != nil {
			return err
		}

		fmt.Println(viper.ConfigFileUsed())

		return nil
	},
}

var listConfigCmd = &cobra.Command{
	Use:               "list",
	Aliases:           []string{"ls"},
	Short:             "List effective config",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		asYaml, err := cmd.Flags().GetBool("yaml")
		if err != nil {
			return err
		}

		if asYaml {
			out, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			fmt.Println(string(out))
		} else {
			keys := viper.AllKeys()
			slices.Sort(keys)
			for _, key := range keys {
				value := viper.Get(key)
				fmt.Printf("%s=%v\n", key, value)
			}
		}

		return nil
	},
}

var getConfigCmd = &cobra.Command{
	Use:               "get key",
	Aliases:           []string{"query", "read"},
	Short:             "Get a config value by key",
	Args:              cobra.ExactArgs(1),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },

	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		_, err := getAppConfig(cmd)
		if err != nil {
			return err
		}

		fmt.Println(viper.Get(args[0]))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	listConfigCmd.Flags().Bool("yaml", false, "Output as YAML")
	configCmd.AddCommand(listConfigCmd, getConfigCmd)
}
