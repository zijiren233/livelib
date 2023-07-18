package cache

import (
	"fmt"

	"github.com/zijiren233/livelib/av"
)

var (
	maxGOPCap    int = 1024
	ErrGopTooBig     = fmt.Errorf("gop to big")
)

type array struct {
	packets    []*av.Packet
	isComplete bool
}

func newArray() *array {
	return &array{
		packets:    make([]*av.Packet, 0, maxGOPCap),
		isComplete: false,
	}
}

func (a *array) reset() {
	a.packets = a.packets[:0]
	a.isComplete = false
}

func (a *array) write(packet *av.Packet) error {
	if len(a.packets) >= maxGOPCap {
		return ErrGopTooBig
	}
	IsKeyFrame := packet.Header.(av.VideoPacketHeader).IsKeyFrame()
	if !a.isComplete && !IsKeyFrame {
		return nil
	}
	if IsKeyFrame {
		a.reset()
		a.isComplete = true
	}
	a.packets = append(a.packets, packet)
	return nil
}

func (a *array) send(w av.WriteCloser) error {
	if !a.isComplete {
		return nil
	}
	for _, packet := range a.packets {
		if err := w.Write(packet); err != nil {
			return err
		}
	}
	return nil
}

type GopCache struct {
	gop *array
}

func NewGopCache() *GopCache {
	return &GopCache{
		gop: newArray(),
	}
}

func (g *GopCache) Write(p *av.Packet) {
	g.gop.write(p)
}

func (g *GopCache) Send(w av.WriteCloser) error {
	return g.gop.send(w)
}
