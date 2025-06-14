package server

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zijiren233/gencontainer/rwmap"
	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/cache"
	"github.com/zijiren233/livelib/protocol/hls"
)

type Channel struct {
	inPublication bool
	players       rwmap.RWMap[av.WriteCloser, *packWriter]

	mu     sync.RWMutex
	closed bool

	hlsOnce sync.Once

	hlsWriter atomic.Pointer[hls.Source]
}

type ChannelConf func(*Channel)

func NewChannel(conf ...ChannelConf) *Channel {
	ch := &Channel{}
	for _, c := range conf {
		c(ch)
	}
	return ch
}

var (
	ErrPusherAlreadyInPublication = errors.New("pusher already in publication")
	ErrPusherNotInPublication     = errors.New("pusher not in publication")
)

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
	if pusher == nil {
		return ErrPusherIsNil
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}

	if c.inPublication {
		c.mu.Unlock()
		return ErrPusherAlreadyInPublication
	}
	c.inPublication = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.kickAllPlayers()
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
		if c.Closed() {
			return nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClosed
	}

	c.kickAllPlayers()
	return nil
}

func (c *Channel) kickAllPlayers() {
	c.players.Range(func(w av.WriteCloser, player *packWriter) bool {
		c.players.Delete(w)
		player.GetWriter().Close()
		return true
	})
}

func (c *Channel) Closed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

func (c *Channel) AddPlayer(w av.WriteCloser) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClosed
	}
	if !c.inPublication {
		return ErrPusherNotInPublication
	}
	_, loaded := c.players.LoadOrStore(w, newPackWriterCloser(w))
	if loaded {
		return errors.New("player already exists")
	}
	return nil
}

func (c *Channel) DelPlayer(w av.WriteCloser) bool {
	pw, loaded := c.players.LoadAndDelete(w)
	if loaded {
		pw.GetWriter().Close()
	}
	return loaded
}

func (c *Channel) InitHlsPlayer(conf ...hls.SourceConf) error {
	c.hlsOnce.Do(func() {
		p := hls.NewSource(conf...)
		c.hlsWriter.Store(p)
		go func() {
			for {
				if err := c.AddPlayer(p); err != nil {
					if errors.Is(err, ErrClosed) {
						p.Close()
						return
					}
					if errors.Is(err, ErrPusherNotInPublication) {
						time.Sleep(time.Second)
					} else {
						runtime.Gosched()
					}
					continue
				}
				_ = p.SendPacket(context.Background())
				p.Close()
				p = hls.NewSource(conf...)
				c.hlsWriter.Store(p)
			}
		}()
	})
	return nil
}

func (c *Channel) HlsPlayer() *hls.Source {
	return c.hlsWriter.Load()
}

func (c *Channel) InitdHlsPlayer() bool {
	return c.hlsWriter.Load() != nil
}

var ErrHlsPlayerNotInit = errors.New("hls player not init")

func (c *Channel) GenM3U8File(tsPath func(tsName string) (tsPath string)) ([]byte, error) {
	if !c.InitdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	return c.HlsPlayer().GetCacheInc().GenM3U8File(tsPath)
}

func (c *Channel) GetTsFile(tsName string) ([]byte, error) {
	if !c.InitdHlsPlayer() {
		return nil, ErrHlsPlayerNotInit
	}
	t, err := c.HlsPlayer().GetCacheInc().GetItem(tsName)
	if err != nil {
		return nil, err
	}
	return t.Data, nil
}
