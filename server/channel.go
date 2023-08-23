package server

import (
	"bytes"
	"errors"
	"sync/atomic"

	"github.com/zijiren233/gencontainer/rwmap"
	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/cache"
	"github.com/zijiren233/livelib/protocol/hls"
)

type Channel struct {
	channelName   string
	inPublication bool
	players       *rwmap.RWMap[av.WriteCloser, *packWriter]

	closed uint32

	hlsWriter *hls.Source
}

func newChannel(channelName string) *Channel {
	return &Channel{
		channelName: channelName,
		players:     &rwmap.RWMap[av.WriteCloser, *packWriter]{},
	}
}

func (c *Channel) InPublication() bool {
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

var (
	ErrPusherIsNil = errors.New("pusher is nil")
	ErrClosed      = errors.New("channel closed")
)

func (c *Channel) PushStart(pusher av.Reader) error {
	if c.Closed() {
		return ErrClosed
	}
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
		if c.Closed() {
			return nil
		}
		p, err := pusher.Read()
		if err != nil {
			return err
		}

		cache.Write(p)

		c.players.Range(func(w av.WriteCloser, player *packWriter) bool {
			if !player.Inited() {
				if err = cache.Send(player.GetWriter()); err != nil {
					c.players.Delete(w)
					player.GetWriter().Close()
				}
				player.Init()
			} else {
				if err = player.GetWriter().Write(p); err != nil {
					c.players.Delete(w)
					player.GetWriter().Close()
				}
			}
			return true
		})
	}
}

func (c *Channel) Close() error {
	if atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		return nil
	}
	c.players.Clear()
	return nil
}

func (c *Channel) Closed() bool {
	return atomic.LoadUint32(&c.closed) == 1
}

func (c *Channel) AddPlayer(w av.WriteCloser) error {
	if c.Closed() {
		return ErrClosed
	}
	c.players.Store(w, newPackWriterCloser(w))
	return nil
}

func (c *Channel) DelPlayer(w av.WriteCloser) error {
	if c.Closed() {
		return ErrClosed
	}
	c.players.Delete(w)
	return nil
}

func (c *Channel) GetPlayers() ([]av.WriteCloser, error) {
	if c.Closed() {
		return nil, ErrClosed
	}
	players := make([]av.WriteCloser, 0)
	c.players.Range(func(w av.WriteCloser, _ *packWriter) bool {
		players = append(players, w)
		return true
	})
	return players, nil
}

func (c *Channel) InitHlsPlayer() error {
	if c.Closed() {
		return ErrClosed
	}
	if c.hlsWriter == nil || c.hlsWriter.Closed() {
		c.hlsWriter = hls.NewSource()
		go c.hlsWriter.SendPacket()
		if err := c.AddPlayer(c.hlsWriter); err != nil {
			return err
		}
	}
	return nil
}

func (c *Channel) InitdHlsPlayer() bool {
	return c.hlsWriter != nil && !c.hlsWriter.Closed()
}

var ErrHlsPlayerNotInit = errors.New("hls player not init")

func (c *Channel) GenM3U8PlayList(tsBashPath string) (*bytes.Buffer, error) {
	if c.Closed() {
		return nil, ErrClosed
	}
	if !c.InitdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	return c.hlsWriter.GetCacheInc().GenM3U8PlayList(tsBashPath), nil
}

func (c *Channel) GetTsFile(tsName string) ([]byte, error) {
	if c.Closed() {
		return nil, ErrClosed
	}
	if !c.InitdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	t, err := c.hlsWriter.GetCacheInc().GetItem(tsName)
	if err != nil {
		return nil, err
	}
	return t.Data, nil
}
