package server

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/zijiren233/gencontainer/rwmap"
	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/cache"
	"github.com/zijiren233/livelib/protocol/hls"
)

type Channel struct {
	inPublication uint32
	players       rwmap.RWMap[av.WriteCloser, *packWriter]

	closed  uint32
	wg      sync.WaitGroup
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
	if !atomic.CompareAndSwapUint32(&c.closed, 0, 1) {
		return ErrClosed
	}

	c.wg.Wait()

	c.players.Range(func(w av.WriteCloser, player *packWriter) bool {
		c.players.Delete(w)
		player.GetWriter().Close()
		return true
	})
	return nil
}

func (c *Channel) Closed() bool {
	return atomic.LoadUint32(&c.closed) == 1
}

func (c *Channel) AddPlayer(w av.WriteCloser) error {
	c.wg.Add(1)
	defer c.wg.Done()
	if c.Closed() {
		return ErrClosed
	}
	_, loaded := c.players.LoadOrStore(w, newPackWriterCloser(w))
	if loaded {
		return errors.New("player already exists")
	}
	return nil
}

func (c *Channel) DelPlayer(w av.WriteCloser) error {
	c.wg.Add(1)
	defer c.wg.Done()
	if c.Closed() {
		return ErrClosed
	}
	pw, loaded := c.players.LoadAndDelete(w)
	if loaded {
		pw.GetWriter().Close()
	}
	return nil
}

func (c *Channel) GetPlayers() ([]av.WriteCloser, error) {
	c.wg.Add(1)
	defer c.wg.Done()
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
	c.wg.Add(1)
	defer c.wg.Done()
	if c.Closed() {
		return ErrClosed
	}
	c.hlsOnce.Do(func() {
		p := hls.NewSource()
		c.hlsWriter.Store(p)
		go func() {
			for {
				if c.Closed() {
					return
				}
				if err := c.AddPlayer(p); err != nil {
					p.Close()
					continue
				}
				p.SendPacket()
				p.Close()
				if c.Closed() {
					return
				}
				p = hls.NewSource()
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
	return c.HlsPlayer().GetCacheInc().GenM3U8File(tsPath), nil
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
