package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/zijiren233/gencontainer/rwmap"
	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/cache"
	"github.com/zijiren233/livelib/protocol/rtmp"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Client struct {
	connClient *core.ConnClient
	method     string

	pulling, inPublication bool

	players *rwmap.RWMap[av.WriteCloser, *packWriter]

	gopSize int
}

var ErrAlreadyDialed = errors.New("already dialed")
var ErrMethodNotSupport = errors.New("method not support")

func Dial(url string, method string) (*Client, error) {
	if method != av.PUBLISH && method != av.PLAY {
		return nil, ErrMethodNotSupport
	}
	c := &Client{method: method, gopSize: 30}
	switch method {
	case av.PUBLISH:
	case av.PLAY:
		c.players = &rwmap.RWMap[av.WriteCloser, *packWriter]{}
	}
	connClient := core.NewConnClient()
	if err := connClient.Start(url, c.method); err != nil {
		return nil, err
	}
	c.connClient = connClient
	return c, nil
}

func (c *Client) Close() error {
	return c.connClient.Close()
}

func (c *Client) Flush() error {
	return c.connClient.Flush()
}

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

func (c *Client) PullStart(ctx context.Context) (err error) {
	if c.method != av.PLAY {
		return ErrMethodNotSupport
	}

	if c.pulling {
		return ErrAlreadyDialed
	}

	c.pulling = true
	defer func() { c.pulling = false }()

	cache := cache.NewCache()

	puller := rtmp.NewReader(c.connClient)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		p, err := puller.Read()
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

func (c *Client) AddPlayer(player av.WriteCloser) (err error) {
	if c.method != av.PLAY {
		return ErrMethodNotSupport
	}
	c.players.Store(player, newPackWriterCloser(player))
	return
}

func (c *Client) DelPlayer(player av.WriteCloser) (err error) {
	if c.method != av.PLAY {
		return ErrMethodNotSupport
	}
	c.players.Delete(player)
	return nil
}

var ErrAlreadyInPublication = errors.New("already in publication")

func (c *Client) PushStart(ctx context.Context, src av.Reader) error {
	if c.method != av.PUBLISH {
		return ErrMethodNotSupport
	}

	if c.inPublication {
		return ErrAlreadyInPublication
	}

	c.inPublication = true
	defer func() { c.inPublication = false }()

	pusher := rtmp.NewWriter(c.connClient)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			p, err := src.Read()
			if err != nil {
				fmt.Printf("err: %v\n", err)
				return
			}
			if err := pusher.Write(p); err != nil {
				return
			}
		}
	}()
	return pusher.SendPacket()
}
