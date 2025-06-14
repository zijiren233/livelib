package hls

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/container/flv"
	"github.com/zijiren233/livelib/container/ts"
	"github.com/zijiren233/livelib/protocol/hls/parser"
)

const (
	videoHZ      = 90000
	aacSampleLen = 1024
	maxQueueNum  = 512

	h264_default_hz uint64 = 90
)

type Source struct {
	seq         int64
	bwriter     *bytes.Buffer
	btswriter   *bytes.Buffer
	demuxer     *flv.Demuxer
	muxer       *ts.Muxer
	pts, dts    uint64
	stat        *status
	align       align
	cache       *audioCache
	tsCache     *TSCache
	tsparser    *parser.CodecParser
	packetQueue chan *av.Packet

	genTsNameFunc func() string

	mu     sync.RWMutex
	closed bool
}

type SourceConf func(*Source)

func WithGenTsNameFunc(f func() string) SourceConf {
	return func(s *Source) {
		s.genTsNameFunc = f
	}
}

func DefaultGenTsNameFunc() string {
	return strconv.FormatInt(time.Now().UnixMicro(), 10)
}

func NewSource(conf ...SourceConf) *Source {
	s := &Source{
		stat:        newStatus(),
		cache:       newAudioCache(),
		demuxer:     flv.NewDemuxer(),
		muxer:       ts.NewMuxer(),
		tsCache:     NewTSCacheItem(),
		tsparser:    parser.NewCodecParser(),
		bwriter:     bytes.NewBuffer(make([]byte, 100*1024)),
		packetQueue: make(chan *av.Packet, maxQueueNum),

		genTsNameFunc: DefaultGenTsNameFunc,
	}
	for _, c := range conf {
		c(s)
	}
	return s
}

func (source *Source) GetCacheInc() *TSCache {
	return source.tsCache
}

func (source *Source) Write(p *av.Packet) (err error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.closed {
		return av.ErrClosed
	}

	for {
		select {
		case source.packetQueue <- p:
			return
		default:
			av.DropPacket(source.packetQueue)
		}
	}
}

func (source *Source) SendPacket(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-source.packetQueue:
			if !ok {
				return nil
			}
			if p.IsMetadata {
				continue
			}
			p = p.DeepClone()
			err := source.demuxer.Demux(p)
			if err != nil {
				if errors.Is(err, flv.ErrAvcEndSEQ) {
					continue
				}
				return err
			}

			compositionTime, isSeq, err := source.parse(p)
			if err != nil || isSeq {
				continue
			}
			if source.btswriter != nil {
				source.stat.update(p.IsVideo, p.TimeStamp)
				source.calcPtsDts(p.IsVideo, p.TimeStamp, uint32(compositionTime))
				source.tsMux(p)
			}
		}
	}
}

// func (source *Source) cleanup() {
// 	source.bwriter = nil
// 	source.btswriter = nil
// 	source.cache = nil
// 	source.tsCache = nil
// }

func (source *Source) Close() error {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.closed {
		return av.ErrClosed
	}
	source.closed = true
	close(source.packetQueue)
	return nil
}

func (source *Source) cut() {
	newf := true
	if source.btswriter == nil {
		source.btswriter = bytes.NewBuffer(nil)
	} else if source.stat.durationMs() >= duration {
		source.flushAudio()

		source.seq++
		source.tsCache.PushItem(NewTSItem(source.genTsNameFunc(), source.stat.durationMs(), source.seq, source.btswriter.Bytes()))

		source.btswriter.Reset()
		source.stat.resetAndNew()
	} else {
		newf = false
	}
	if newf {
		source.btswriter.Write(source.muxer.PAT())
		source.btswriter.Write(source.muxer.PMT(av.SOUND_AAC, true))
	}
}

func (source *Source) parse(p *av.Packet) (int32, bool, error) {
	var compositionTime int32
	var ah av.AudioPacketHeader
	var vh av.VideoPacketHeader
	if p.IsVideo {
		vh = p.Header.(av.VideoPacketHeader)
		if vh.CodecID() != av.CODEC_AVC {
			return compositionTime, false, ErrNoSupportVideoCodec
		}
		compositionTime = vh.CompositionTime()
		if vh.IsKeyFrame() && vh.IsSeq() {
			return compositionTime, true, source.tsparser.Parse(p, source.bwriter)
		}
	} else {
		ah = p.Header.(av.AudioPacketHeader)
		if ah.SoundFormat() != av.SOUND_AAC {
			return compositionTime, false, ErrNoSupportAudioCodec
		}
		if ah.AACPacketType() == av.AAC_SEQHDR {
			return compositionTime, true, source.tsparser.Parse(p, source.bwriter)
		}
	}
	source.bwriter.Reset()
	if err := source.tsparser.Parse(p, source.bwriter); err != nil {
		return compositionTime, false, err
	}
	p.Data = source.bwriter.Bytes()

	if p.IsVideo && vh.IsKeyFrame() {
		source.cut()
	}
	return compositionTime, false, nil
}

func (source *Source) calcPtsDts(isVideo bool, ts, compositionTs uint32) {
	source.dts = uint64(ts) * h264_default_hz
	if isVideo {
		source.pts = source.dts + uint64(compositionTs)*h264_default_hz
	} else {
		sampleRate, _ := source.tsparser.SampleRate()
		source.align.align(&source.dts, uint32(videoHZ*aacSampleLen/sampleRate))
		source.pts = source.dts
	}
}

func (source *Source) flushAudio() error {
	return source.muxAudio(1)
}

func (source *Source) muxAudio(limit byte) error {
	if source.cache.CacheNum() < limit {
		return nil
	}
	var p av.Packet
	_, pts, buf := source.cache.GetFrame()
	p.Data = buf
	p.TimeStamp = uint32(pts / h264_default_hz)
	return source.muxer.Mux(&p, source.btswriter)
}

func (source *Source) tsMux(p *av.Packet) error {
	if p.IsVideo {
		return source.muxer.Mux(p, source.btswriter)
	} else {
		source.cache.Cache(p.Data, source.pts)
		return source.muxAudio(cache_max_frames)
	}
}
