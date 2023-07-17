package cache

import (
	"github.com/zijiren233/livelib/av"
)

const (
	SetDataFrame string = "@setDataFrame"
	OnMetaData   string = "onMetaData"
)

type SpecialCache struct {
	p          *av.Packet
	isComplete bool
}

func NewSpecialCache() *SpecialCache {
	return &SpecialCache{}
}

func (s *SpecialCache) Write(p *av.Packet) {
	s.isComplete = true
	s.p = p
}

func (s *SpecialCache) Send(w av.WriteCloser) error {
	if !s.isComplete {
		return nil
	}

	return w.Write(s.p)
}
