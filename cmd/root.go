package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/trugamr/wol/config"
)

var (
	cfg = config.NewConfig()
	// cfgFile is the explicit config path from --config; empty means search the
	// default locations.
	cfgFile string
	// configSources records which sources contributed to the loaded config, so
	// commands can report them (see serve).
	configSources []string
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path (overrides the default search locations)")
}

var rootCmd = &cobra.Command{
	Use:   "wol",
	Short: "Discover and wake up devices on the network",
	Long:  "Discover devices on the network and wake them by sending magic Wake-On-LAN packets",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		sources, err := cfg.Load(cfgFile)
		if err != nil {
			return err
		}
		configSources = sources
		return nil
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
