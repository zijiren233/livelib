package cmd

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math/rand"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/soheilhy/cmux"
	"github.com/spf13/cobra"
	"github.com/zijiren233/gencontainer/rwmap"
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
	channels := rwmap.RWMap[string, *server.Channel]{}
	s := server.NewRtmpServer(func(ReqAppName, ReqChannelName string, IsPublisher bool) (*server.Channel, error) {
		c, _ := channels.LoadOrStore(ReqAppName, server.NewChannel())
		return c, c.InitHlsPlayer()
	})
	go s.Serve(tcp)
	if flags.Dev {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	e := gin.Default()
	utils.Cors(e)
	e.GET("/:app/*channel", func(ctx *gin.Context) {
		appName := ctx.Param("app")
		channelStr := strings.Trim(ctx.Param("channel"), "/")
		channelSplitd := strings.Split(channelStr, "/")
		fileName := channelSplitd[0]
		fileExt := path.Ext(channelStr)
		channelName := strings.TrimSuffix(fileName, fileExt)

		channel, ok := channels.Load(appName)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"error": "app not found",
			})
			return
		}
		switch fileExt {
		case ".flv":
			w := httpflv.NewHttpFLVWriter(ctx.Writer)
			defer w.Close()
			channel.AddPlayer(w)
			w.SendPacket()
		case ".m3u8":
			b, err := channel.GenM3U8File(func(tsName string) (tsPath string) {
				return fmt.Sprintf("/%s/%s/%s.%s", appName, channelName, tsName, ctx.DefaultQuery("t", "ts"))
			})
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"error": err.Error(),
				})
				return
			}
			ctx.Data(http.StatusOK, hls.M3U8ContentType, b)
		case ".ts":
			b, err := channel.GetTsFile(channelSplitd[1])
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"error": err.Error(),
				})
				return
			}
			ctx.Data(http.StatusOK, hls.TSContentType, b)
		case ".png":
			b, err := channel.GetTsFile(strings.TrimRight(channelSplitd[1], ".png"))
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					"error": err.Error(),
				})
				return
			}
			img := image.NewGray(image.Rect(0, 0, 1, 1))
			img.Set(1, 1, color.Gray{uint8(rand.Intn(255))})
			f := bytes.NewBuffer(nil)
			err = png.Encode(f, img)
			if err != nil {
				panic(err)
			}
			ctx.Data(http.StatusOK, "image/png", f.Bytes())
			ctx.Writer.Write(b)
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
