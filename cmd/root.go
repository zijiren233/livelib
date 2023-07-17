package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zijiren233/livelib/cmd/flags"
)

var RootCmd = &cobra.Command{
	Use:   "livelib",
	Short: "livelib",
	Long:  `livelib https://github.com/zijiren233/livelib`,
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "debug mode")
}
