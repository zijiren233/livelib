package cmd

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/soheilhy/cmux"
	"github.com/spf13/cobra"
	"github.com/zijiren233/livelib/cmd/flags"
	"github.com/zijiren233/livelib/protocol/hls"
	"github.com/zijiren233/livelib/protocol/httpflv"
	"github.com/zijiren233/livelib/server"
	"github.com/zijiren233/livelib/utils"
)

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Start livelib server",
	Long:  `Start livelib server`,
	Run:   Server,
}

func Server(cmd *cobra.Command, args []string) {
	host := fmt.Sprintf("%s:%d", flags.Listen, flags.Port)
	fmt.Printf("Run on tcp://%s\nRtmp: rtmp://%s/{app}/{channel}\nWebAPI: http://%s/{app}/{channel}\n", host, host, host)
	listener, err := net.Listen("tcp", host)
	if err != nil {
		log.Panic(err)
	}
	muxer := cmux.New(listener)
	httpl := muxer.Match(cmux.HTTP1Fast())
	tcp := muxer.Match(cmux.Any())
	s := server.NewRtmpServer(server.WithInitHlsPlayer(true))
	go s.Serve(tcp)
	if flags.Dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	e := gin.Default()
	utils.Cors(e)
	e.GET("/:app/*channel", func(c *gin.Context) {
		appName := c.Param("app")
		channelStr := strings.Trim(c.Param("channel"), "/")
		channelSplitd := strings.Split(channelStr, "/")
		fileName := channelSplitd[0]
		fileExt := path.Ext(channelStr)
		channelName := strings.TrimSuffix(fileName, fileExt)
		channel, err := s.GetChannelWithApp(appName, channelName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
			return
		}
		switch fileExt {
		case ".flv":
			w := httpflv.NewFLVWriter(c, c.Writer)
			defer w.Close()
			channel.AddPlayer(w)
			w.SendPacket()
		case ".m3u8":
			b, err := channel.GenM3U8PlayList(fmt.Sprintf("/%s/%s", appName, channelName))
			if err != nil {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"error": err.Error(),
				})
				return
			}
			c.Data(http.StatusOK, hls.M3U8ContentType, b.Bytes())
		case ".ts":
			b, err := channel.GetTsFile(channelSplitd[1])
			if err != nil {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"error": err.Error(),
				})
				return
			}
			c.Data(http.StatusOK, hls.TSContentType, b)
		}
	})
	go http.Serve(httpl, e.Handler())

	muxer.Serve()
}

func init() {
	RootCmd.AddCommand(ServerCmd)
	ServerCmd.Flags().StringVarP(&flags.Listen, "listen", "l", "127.0.0.1", "address to listen on")
	ServerCmd.Flags().Uint16VarP(&flags.Port, "port", "p", 1935, "port to listen on")
}
