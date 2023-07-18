package hls

import (
	"bytes"
	"context"
	"fmt"
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
	*av.RWBaser
	seq         int
	bwriter     *bytes.Buffer
	btswriter   *bytes.Buffer
	demuxer     *flv.Demuxer
	muxer       *ts.Muxer
	pts, dts    uint64
	stat        *status
	align       *align
	cache       *audioCache
	tsCache     *TSCacheItem
	tsparser    *parser.CodecParser
	packetQueue chan *av.Packet

	ctx    context.Context
	cancel context.CancelFunc
}

func NewSource(ctx context.Context) *Source {
	s := &Source{
		align:       &align{},
		stat:        newStatus(),
		RWBaser:     av.NewRWBaser(),
		cache:       newAudioCache(),
		demuxer:     flv.NewDemuxer(),
		muxer:       ts.NewMuxer(),
		tsCache:     NewTSCacheItem(),
		tsparser:    parser.NewCodecParser(),
		bwriter:     bytes.NewBuffer(make([]byte, 100*1024)),
		packetQueue: make(chan *av.Packet, maxQueueNum),
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	return s
}

func (source *Source) GetCacheInc() *TSCacheItem {
	return source.tsCache
}

func (source *Source) Write(p *av.Packet) (err error) {
	select {
	case <-source.ctx.Done():
		return source.ctx.Err()
	case source.packetQueue <- p:
	default:
		av.DropPacket(source.packetQueue)
	}
	return
}

func (source *Source) SendPacket(ClearCacheWhenClosed bool) error {
	var p *av.Packet
	var ok bool
	for {
		select {
		case <-source.ctx.Done():
			if !ClearCacheWhenClosed || len(source.packetQueue) == 0 {
				return nil
			}
			p, ok = <-source.packetQueue
		case p, ok = <-source.packetQueue:
			if !ClearCacheWhenClosed {
				return nil
			}
		}
		if !ok {
			return nil
		}
		if p.IsMetadata {
			continue
		}
		p = p.NewPacketData()
		err := source.demuxer.Demux(p)
		if err != nil {
			fmt.Printf("err: %v\n", err)
			if err == flv.ErrAvcEndSEQ {
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

func (source *Source) cleanup() {
	source.bwriter = nil
	source.btswriter = nil
	source.cache = nil
	source.tsCache = nil
}

func (source *Source) Close() error {
	source.cancel()
	source.cleanup()
	close(source.packetQueue)
	return source.ctx.Err()
}

func (source *Source) Closed() bool {
	select {
	case <-source.ctx.Done():
		return true
	default:
		return false
	}
}

func (source *Source) cut() {
	newf := true
	if source.btswriter == nil {
		source.btswriter = bytes.NewBuffer(nil)
	} else if source.stat.durationMs() >= duration {
		source.flushAudio()

		source.seq++
		filename := fmt.Sprint(time.Now().Unix())
		item := NewTSItem(filename, int(source.stat.durationMs()), source.seq, source.btswriter.Bytes())
		source.tsCache.PushItem(item)

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
		if vh.CodecID() != av.VIDEO_H264 {
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
