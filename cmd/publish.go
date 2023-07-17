package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/client"
	"github.com/zijiren233/livelib/cmd/flags"
	"github.com/zijiren233/livelib/container/flv"
)

var PublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Start livelib play",
	Long:  `Start livelib play`,
	Run:   Publish,
}

func Publish(cmd *cobra.Command, args []string) {
	c, err := client.Dial(flags.Dial, av.PUBLISH)
	if err != nil {
		panic(err)
	}

	file, err := os.OpenFile(flags.FilePath, os.O_RDONLY, os.ModePerm)
	if err != nil {
		panic(err)
	}

	r := flv.NewReader(file)

	if err := c.PushStart(context.Background(), r); err != nil {
		panic(err)
	}
}

func init() {
	ClientCmd.AddCommand(PublishCmd)
	PublishCmd.Flags().StringVarP(&flags.FilePath, "file", "f", "", "publish flv file to server")
}
