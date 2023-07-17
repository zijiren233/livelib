package rtmprelay

import (
	"bytes"
	"fmt"
	"io"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

var (
	STOP_CTRL = "RTMPRELAY_STOP"
)

type RtmpRelay struct {
	PlayUrl              string
	PublishUrl           string
	cs_chan              chan *core.ChunkStream
	sndctrl_chan         chan string
	connectPlayClient    *core.ConnClient
	connectPublishClient *core.ConnClient
	startflag            bool
}

func NewRtmpRelay(playurl *string, publishurl *string) *RtmpRelay {
	return &RtmpRelay{
		PlayUrl:              *playurl,
		PublishUrl:           *publishurl,
		cs_chan:              make(chan *core.ChunkStream, 500),
		sndctrl_chan:         make(chan string),
		connectPlayClient:    nil,
		connectPublishClient: nil,
		startflag:            false,
	}
}

func (r *RtmpRelay) rcvPlayChunkStream() {
	for {

		if !r.startflag {
			r.connectPlayClient.Close()
			break
		}
		rc, err := r.connectPlayClient.Read()

		if err != nil && err == io.EOF {
			break
		}
		switch rc.TypeID {
		case 20, 17:
			reader := bytes.NewReader(rc.Data)
			r.connectPlayClient.DecodeBatch(reader, amf.AMF0)
		case 18:
			r.cs_chan <- rc
		case 8, 9:
			r.cs_chan <- rc
		}
	}
}

func (r *RtmpRelay) sendPublishChunkStream() {
	for {
		select {
		case rc := <-r.cs_chan:
			r.connectPublishClient.Write(rc)
		case ctrlcmd := <-r.sndctrl_chan:
			if ctrlcmd == STOP_CTRL {
				r.connectPublishClient.Close()
				return
			}
		}
	}
}

func (r *RtmpRelay) Start() error {
	if r.startflag {
		return fmt.Errorf("The rtmprelay already started, playurl=%s, publishurl=%s\n", r.PlayUrl, r.PublishUrl)
	}

	r.connectPlayClient = core.NewConnClient()
	r.connectPublishClient = core.NewConnClient()

	err := r.connectPlayClient.Start(r.PlayUrl, av.PLAY)
	if err != nil {
		return err
	}

	err = r.connectPublishClient.Start(r.PublishUrl, av.PUBLISH)
	if err != nil {
		r.connectPlayClient.Close()
		return err
	}

	r.startflag = true
	go r.rcvPlayChunkStream()
	go r.sendPublishChunkStream()

	return nil
}

func (r *RtmpRelay) Stop() {
	if !r.startflag {
		return
	}

	r.startflag = false
	r.sndctrl_chan <- STOP_CTRL
}
