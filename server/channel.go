package server

import (
	"bytes"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/zijiren233/gencontainer/rwmap"
	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/cache"
	"github.com/zijiren233/livelib/protocol/hls"
)

type Channel struct {
	channelName   string
	inPublication uint32
	players       *rwmap.RWMap[av.WriteCloser, *packWriter]

	closed bool
	lock   *sync.RWMutex

	hlsWriter *hls.Source
}

func newChannel(channelName string) *Channel {
	return &Channel{
		channelName: channelName,
		lock:        &sync.RWMutex{},
		players:     &rwmap.RWMap[av.WriteCloser, *packWriter]{},
	}
}

func (c *Channel) InPublication() bool {
	return atomic.LoadUint32(&c.inPublication) == 1
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
	if !atomic.CompareAndSwapUint32(&c.inPublication, 0, 1) {
		return ErrPusherAlreadyInPublication
	}
	defer atomic.CompareAndSwapUint32(&c.inPublication, 1, 0)

	if pusher == nil {
		return ErrPusherIsNil
	}

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
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.closed {
		return ErrClosed
	}
	c.closed = true
	c.players.Range(func(w av.WriteCloser, player *packWriter) bool {
		c.players.Delete(w)
		player.GetWriter().Close()
		return true
	})
	return nil
}

func (c *Channel) Closed() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.closed
}

func (c *Channel) AddPlayer(w av.WriteCloser) error {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.closed {
		return ErrClosed
	}
	c.players.Store(w, newPackWriterCloser(w))
	return nil
}

func (c *Channel) addPlayer(w av.WriteCloser) error {
	if c.closed {
		return ErrClosed
	}
	c.players.Store(w, newPackWriterCloser(w))
	return nil
}

func (c *Channel) DelPlayer(w av.WriteCloser) error {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.closed {
		return ErrClosed
	}
	c.players.Delete(w)
	return nil
}

func (c *Channel) GetPlayers() ([]av.WriteCloser, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.closed {
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
	c.lock.RLock()
	if c.closed {
		c.lock.RUnlock()
		return ErrClosed
	}
	if !c.initdHlsPlayer() {
		c.lock.RUnlock()

		c.lock.Lock()
		defer c.lock.Unlock()
		if c.initdHlsPlayer() {
			return nil
		}
		c.hlsWriter = hls.NewSource()
		go c.hlsWriter.SendPacket()
		if err := c.addPlayer(c.hlsWriter); err != nil {
			c.hlsWriter.Close()
			return err
		}
		return nil
	}
	c.lock.RUnlock()
	return nil
}

func (c *Channel) InitdHlsPlayer() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.initdHlsPlayer()
}

func (c *Channel) initdHlsPlayer() bool {
	return c.hlsWriter != nil && !c.hlsWriter.Closed()
}

var ErrHlsPlayerNotInit = errors.New("hls player not init")

func (c *Channel) GenM3U8PlayList(tsBashPath string) (*bytes.Buffer, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.closed {
		return nil, ErrClosed
	}
	if !c.initdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	return c.hlsWriter.GetCacheInc().GenM3U8PlayList(tsBashPath), nil
}

func (c *Channel) GetTsFile(tsName string) ([]byte, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.closed {
		return nil, ErrClosed
	}
	if !c.initdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	t, err := c.hlsWriter.GetCacheInc().GetItem(tsName)
	if err != nil {
		return nil, err
	}
	return t.Data, nil
}
