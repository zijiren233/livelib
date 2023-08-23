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

var PlayCmd = &cobra.Command{
	Use:   "play",
	Short: "Start livelib play",
	Long:  `Start livelib play`,
	Run:   Play,
}

func Play(cmd *cobra.Command, args []string) {
	c, err := client.Dial(flags.Dial, av.PLAY)
	if err != nil {
		panic(err)
	}

	file, err := os.Create(flags.FilePath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	w := flv.NewWriter(file)

	if err := c.AddPlayer(w); err != nil {
		panic(err)
	}

	if err := c.PullStart(context.Background()); err != nil {
		panic(err)
	}
}

func init() {
	ClientCmd.AddCommand(PlayCmd)
	PlayCmd.Flags().StringVarP(&flags.FilePath, "file", "f", "", "flv save to filepath")
}
