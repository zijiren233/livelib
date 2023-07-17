package client

import (
	"container/list"
	"context"
	"errors"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/av/cache"
	"github.com/zijiren233/livelib/protocol/rtmp"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Client struct {
	connClient *core.ConnClient
	method     string

	pulling, inPublication bool

	playerList *list.List
	cache      *cache.Cache

	pusher *rtmp.Writer
	puller *rtmp.Reader
}

func NewRtmpClient(method string) (*Client, error) {
	if method != av.PUBLISH && method != av.PLAY {
		return nil, ErrMethodNotSupport
	}
	c := &Client{method: method}
	switch method {
	case av.PUBLISH:
	case av.PLAY:
		c.cache = cache.NewCache()
		c.playerList = list.New()
	}
	return c, nil
}

var ErrAlreadyDialed = errors.New("already dialed")
var ErrMethodNotSupport = errors.New("method not support")

func (c *Client) Dial(url string) error {
	connClient := core.NewConnClient()
	if err := connClient.Start(url, c.method); err != nil {
		return err
	}
	c.connClient = connClient
	switch c.method {
	case av.PUBLISH:
		c.pusher = rtmp.NewWriter(context.Background(), c.connClient)
	case av.PLAY:
		c.puller = rtmp.NewReader(context.Background(), c.connClient)
	}
	return nil
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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		p, err := c.puller.Read()
		if err != nil {
			return err
		}

		c.cache.Write(p)

		for e := c.playerList.Front(); e != nil; e = e.Next() {
			player, ok := e.Value.(*packWriter)
			if !ok {
				c.playerList.Remove(e)
				continue
			}
			if !player.Inited() {
				if err = c.cache.Send(player.GetWriter()); err != nil {
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

func (c *Client) AddPlayer(player av.WriteCloser) (e *list.Element, err error) {
	if c.method != av.PLAY {
		return nil, ErrMethodNotSupport
	}
	e = c.playerList.PushBack(newPackWriterCloser(player))
	return
}

func (c *Client) DelPlayer(e *list.Element) (a any, err error) {
	if c.method != av.PLAY {
		return nil, ErrMethodNotSupport
	}
	return c.playerList.Remove(e), nil
}

var ErrAlreadyInPublication = errors.New("already in publication")

func (c *Client) PushStart(ctx context.Context, src av.ReadCloser) error {
	if c.method != av.PUBLISH {
		return ErrMethodNotSupport
	}

	if c.inPublication {
		return ErrAlreadyInPublication
	}

	c.inPublication = true
	defer func() { c.inPublication = false }()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		p, err := src.Read()
		if err != nil {
			return err
		}
		if err := c.pusher.Write(p); err != nil {
			return err
		}
	}
}
