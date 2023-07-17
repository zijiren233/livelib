package cmd

import (
	"github.com/spf13/cobra"
)

var PublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Start livelib play",
	Long:  `Start livelib play`,
	Run:   Publish,
}

func Publish(cmd *cobra.Command, args []string) {
	// c, err := client.NewRtmpClient(av.PUBLISH)
	// if err != nil {
	// 	panic(err)
	// }

	// if err := c.Dial(flags.Dial); err != nil {
	// 	panic(err)
	// }

	// file, err := os.OpenFile("./test.flv", os.O_RDONLY, os.ModePerm)
	// if err != nil {
	// 	panic(err)
	// }

	// c.PushStart(file)
}

func init() {
	ClientCmd.AddCommand(PublishCmd)
}
