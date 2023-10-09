package hls

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/container/flv"
	"github.com/zijiren233/livelib/container/ts"
	"github.com/zijiren233/livelib/protocol/hls/parser"
	"github.com/zijiren233/livelib/utils"
)

const (
	videoHZ      = 90000
	aacSampleLen = 1024
	maxQueueNum  = 512

	h264_default_hz uint64 = 90
)

type Source struct {
	seq         int64
	t           utils.Timestamp
	bwriter     *bytes.Buffer
	btswriter   *bytes.Buffer
	demuxer     *flv.Demuxer
	muxer       *ts.Muxer
	pts, dts    uint64
	stat        *status
	align       *align
	cache       *audioCache
	tsCache     *TSCache
	tsparser    *parser.CodecParser
	packetQueue chan *av.Packet

	closed uint32
	wg     sync.WaitGroup
}

func NewSource() *Source {
	s := &Source{
		align:       new(align),
		btswriter:   bytes.NewBuffer(nil),
		stat:        newStatus(),
		cache:       newAudioCache(),
		demuxer:     flv.NewDemuxer(),
		muxer:       ts.NewMuxer(),
		tsCache:     NewTSCacheItem(),
		tsparser:    parser.NewCodecParser(),
		bwriter:     bytes.NewBuffer(make([]byte, 100*1024)),
		packetQueue: make(chan *av.Packet, maxQueueNum),
	}
	return s
}

func (source *Source) GetCacheInc() *TSCache {
	return source.tsCache
}

func (source *Source) Write(p *av.Packet) (err error) {
	source.wg.Add(1)
	defer source.wg.Done()

	if source.Closed() {
		return av.ErrClosed
	}

	p = p.Clone()
	p.TimeStamp = source.t.RecTimeStamp(p.TimeStamp)

	select {
	case source.packetQueue <- p:
	default:
		av.DropPacket(source.packetQueue)
	}
	return
}

func (source *Source) SendPacket() error {
	for p := range source.packetQueue {
		if p.IsMetadata {
			continue
		}
		p = p.DeepClone()
		err := source.demuxer.Demux(p)
		if err != nil {
			if err == flv.ErrAvcEndSEQ {
				continue
			}
			return err
		}

		compositionTime, isSeq, err := source.parse(p)
		if err != nil || isSeq {
			continue
		}
		source.stat.update(p.IsVideo, p.TimeStamp)
		source.calcPtsDts(p.IsVideo, p.TimeStamp, uint32(compositionTime))
		source.tsMux(p)
	}
	return nil
}

// func (source *Source) cleanup() {
// 	source.bwriter = nil
// 	source.btswriter = nil
// 	source.cache = nil
// 	source.tsCache = nil
// }

func (source *Source) Close() error {
	if !atomic.CompareAndSwapUint32(&source.closed, 0, 1) {
		return av.ErrClosed
	}
	source.wg.Wait()
	// source.cleanup()
	close(source.packetQueue)
	return nil
}

func (source *Source) Closed() bool {
	return atomic.LoadUint32(&source.closed) == 1
}

func (source *Source) cut() {
	newf := true
	if source.stat.durationMs() >= duration {
		source.flushAudio()

		source.seq++
		filename := fmt.Sprint(time.Now().UnixMilli())
		source.tsCache.PushItem(NewTSItem(filename, source.stat.durationMs(), source.seq, source.btswriter.Bytes()))

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
