package server

import (
	"bytes"
	"context"
	"errors"

	"github.com/zijiren233/gencontainer/dllist"
	"github.com/zijiren233/gencontainer/vec"
	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/cache"
	"github.com/zijiren233/livelib/protocol/hls"
)

type channel struct {
	channelName   string
	inPublication bool
	playerList    *dllist.Dllist[*packWriter]

	closed bool

	hlsWriter *hls.Source
}

func newChannel(channelName string) *channel {
	return &channel{
		channelName: channelName,
		playerList:  dllist.New[*packWriter](),
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

var (
	ErrPusherIsNil = errors.New("pusher is nil")
	ErrClosed      = errors.New("channel closed")
)

func (c *channel) PushStart(ctx context.Context, pusher av.Reader) error {
	if c.closed {
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

	needRemove := vec.New[*dllist.Element[*packWriter]]()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if c.closed {
				return ErrClosed
			}
		}

		p, err := pusher.Read()
		if err != nil {
			return err
		}

		cache.Write(p)

		for e := c.playerList.Front(); e != nil; e = e.Next() {
			if !e.Value.Inited() {
				if err = cache.Send(e.Value.GetWriter()); err != nil {
					needRemove.Push(e)
					e.Value.GetWriter().Close()
					continue
				}
				e.Value.Init()
			} else {
				if err = e.Value.GetWriter().Write(p); err != nil {
					needRemove.Push(e)
					e.Value.GetWriter().Close()
					continue
				}
			}
		}
		needRemove.Range(func(i int, e *dllist.Element[*packWriter]) (Continue bool) {
			c.playerList.Remove(e)
			return true
		})
		needRemove.Clear()
		if needRemove.Cap() > 1024 {
			needRemove.Clip()
		}
	}
}

func (c *channel) Close() error {
	if c.closed {
		return ErrClosed
	}
	c.closed = true
	for e := c.playerList.Front(); e != nil; e = e.Next() {
		e.Value.GetWriter().Close()
	}
	c.playerList = nil
	return nil
}

func (c *channel) Closed() bool {
	return c.closed
}

func (c *channel) AddPlayer(w av.WriteCloser) (*dllist.Element[*packWriter], error) {
	if c.closed {
		return nil, ErrClosed
	}
	player := newPackWriterCloser(w)
	e := c.playerList.PushFront(player)
	return e, nil
}

func (c *channel) DelPlayer(e *dllist.Element[*packWriter]) (*packWriter, error) {
	if c.closed {
		return nil, ErrClosed
	}
	return c.playerList.Remove(e), nil
}

func (c *channel) GetPlayers() ([]av.WriteCloser, error) {
	if c.closed {
		return nil, ErrClosed
	}
	players := make([]av.WriteCloser, 0, c.playerList.Len())
	for e := c.playerList.Front(); e != nil; e = e.Next() {
		players = append(players, e.Value.GetWriter())
	}
	return players, nil
}

func (c *channel) InitHlsPlayer() error {
	if c.closed {
		return ErrClosed
	}
	if c.hlsWriter == nil || c.hlsWriter.Closed() {
		c.hlsWriter = hls.NewSource(context.Background())
		go c.hlsWriter.SendPacket(true)
		c.AddPlayer(c.hlsWriter)
	}
	return nil
}

func (c *channel) InitdHlsPlayer() bool {
	return c.hlsWriter != nil && !c.hlsWriter.Closed()
}

var ErrHlsPlayerNotInit = errors.New("hls player not init")

func (c *channel) GenM3U8PlayList(tsBashPath string) (*bytes.Buffer, error) {
	if c.closed {
		return nil, ErrClosed
	}
	if !c.InitdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	return c.hlsWriter.GetCacheInc().GenM3U8PlayList(tsBashPath), nil
}

func (c *channel) GetTsFile(tsName string) ([]byte, error) {
	if c.closed {
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
