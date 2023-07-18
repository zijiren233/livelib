package server

import (
	"bytes"
	"container/list"
	"context"
	"errors"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/cache"
	"github.com/zijiren233/livelib/protocol/hls"
)

type channel struct {
	channelName   string
	inPublication bool
	playerList    *list.List

	hlsWriter *hls.Source
}

func newChannel(channelName string) *channel {
	return &channel{
		channelName: channelName,
		playerList:  list.New(),
	}
}

func (c *channel) InPublication() bool {
	return c.inPublication
}

var ErrPusherAlreadyInPublication = errors.New("pusher already in publication")
var ErrPusherNotInPublication = errors.New("pusher not in publication")

type packWriter struct {
	init bool
	w    av.WriteCloser
}

func newPackWriterCloser(w av.WriteCloser) *packWriter {
	return &packWriter{
		w: w,
	}
}

func (p *packWriter) GetWriter() av.WriteCloser {
	return p.w
}

func (p *packWriter) Init() {
	p.init = true
}

func (p *packWriter) Inited() bool {
	return p.init
}

var ErrPusherIsNil = errors.New("pusher is nil")

func (c *channel) PushStart(ctx context.Context, pusher av.Reader) error {
	if c.inPublication {
		return ErrPusherAlreadyInPublication
	}

	if pusher == nil {
		return ErrPusherIsNil
	}

	c.inPublication = true
	defer func() {
		c.inPublication = false
	}()

	cache := cache.NewCache()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		p, err := pusher.Read()
		if err != nil {
			return err
		}

		cache.Write(p)

		for e := c.playerList.Front(); e != nil; e = e.Next() {
			player, ok := e.Value.(*packWriter)
			if !ok {
				c.playerList.Remove(e)
				continue
			}
			if !player.Inited() {
				if err = cache.Send(player.GetWriter()); err != nil {
					c.playerList.Remove(e)
					player.GetWriter().Close()
					continue
				}
				player.Init()
			} else {
				if err = player.GetWriter().Write(p); err != nil {
					c.playerList.Remove(e)
					player.GetWriter().Close()
					continue
				}
			}
		}
	}
}

func (c *channel) AddPlayer(w av.WriteCloser) *list.Element {
	player := newPackWriterCloser(w)
	e := c.playerList.PushFront(player)
	return e
}

func (c *channel) DelPlayer(e *list.Element) any {
	return c.playerList.Remove(e)
}

func (c *channel) InitHlsPlayer() {
	if c.hlsWriter == nil || c.hlsWriter.Closed() {
		c.hlsWriter = hls.NewSource(context.Background())
		c.AddPlayer(c.hlsWriter)
		go c.hlsWriter.SendPacket(true)
	}
}

func (c *channel) InitdHlsPlayer() bool {
	return c.hlsWriter != nil && !c.hlsWriter.Closed()
}

var ErrHlsPlayerNotInit = errors.New("hls player not init")

func (c *channel) GenM3U8PlayList(tsBashPath string) (*bytes.Buffer, error) {
	if !c.InitdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	return c.hlsWriter.GetCacheInc().GenM3U8PlayList(tsBashPath), nil
}

func (c *channel) GetTsFile(tsName string) ([]byte, error) {
	if !c.InitdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	t, err := c.hlsWriter.GetCacheInc().GetItem(tsName)
	if err != nil {
		return nil, err
	}
	return t.Data, nil
}
