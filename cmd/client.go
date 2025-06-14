package cmd

import (
	"github.com/spf13/cobra"
	"github.com/zijiren233/livelib/cmd/flags"
)

var ClientCmd = &cobra.Command{
	Use:   "client",
	Short: "Start livelib client",
	Long:  `Start livelib client`,
}

func init() {
	RootCmd.AddCommand(ClientCmd)
	ClientCmd.PersistentFlags().
		StringVar(&flags.Dial, "dial", "rtmp://127.0.0.1:1935/app/channel", "dial to server")
}
