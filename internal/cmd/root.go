package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var rootOsExit = os.Exit

var rootCmd = &cobra.Command{
	Use:   "aircover",
	Short: "Air Cover is a Spinitron integration for DJ sub-request management",
	Long:  `Air Cover is a Spinitron integration for DJ sub-request management.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		rootOsExit(1)
	}
}
