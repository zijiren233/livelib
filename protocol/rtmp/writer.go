package rtmp

import (
	"context"
	"reflect"
	"time"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Writer struct {
	ctx    context.Context
	cancel context.CancelFunc
	*av.RWBaser
	conn        ChunkWriter
	packetQueue chan *av.Packet
	WriteBWInfo StaticsBW
}

func NewWriter(ctx context.Context, conn ChunkWriter) *Writer {
	w := &Writer{
		conn:        conn,
		RWBaser:     av.NewRWBaser(),
		packetQueue: make(chan *av.Packet, maxQueueNum),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}
	w.ctx, w.cancel = context.WithCancel(ctx)

	return w
}

func (v *Writer) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	nowInMS := int64(time.Now().UnixNano() / 1e6)

	v.WriteBWInfo.StreamId = streamid
	if isVideoFlag {
		v.WriteBWInfo.VideoDatainBytes = v.WriteBWInfo.VideoDatainBytes + length
	} else {
		v.WriteBWInfo.AudioDatainBytes = v.WriteBWInfo.AudioDatainBytes + length
	}

	if v.WriteBWInfo.LastTimestamp == 0 {
		v.WriteBWInfo.LastTimestamp = nowInMS
	} else if (nowInMS - v.WriteBWInfo.LastTimestamp) >= SAVE_STATICS_INTERVAL {
		diffTimestamp := (nowInMS - v.WriteBWInfo.LastTimestamp) / 1000

		v.WriteBWInfo.VideoSpeedInBytesperMS = (v.WriteBWInfo.VideoDatainBytes - v.WriteBWInfo.LastVideoDatainBytes) * 8 / uint64(diffTimestamp) / 1000
		v.WriteBWInfo.AudioSpeedInBytesperMS = (v.WriteBWInfo.AudioDatainBytes - v.WriteBWInfo.LastAudioDatainBytes) * 8 / uint64(diffTimestamp) / 1000

		v.WriteBWInfo.LastVideoDatainBytes = v.WriteBWInfo.VideoDatainBytes
		v.WriteBWInfo.LastAudioDatainBytes = v.WriteBWInfo.AudioDatainBytes
		v.WriteBWInfo.LastTimestamp = nowInMS
	}
}

func (v *Writer) Write(p *av.Packet) (err error) {
	select {
	case <-v.ctx.Done():
		return v.ctx.Err()
	case v.packetQueue <- p:
	default:
		av.DropPacket(v.packetQueue)
	}

	return
}

func (w *Writer) SendPacket(ClearCacheWhenClosed bool) error {
	Flush := reflect.ValueOf(w.conn).MethodByName("Flush")
	var cs = new(core.ChunkStream)
	var p *av.Packet
	var ok bool
	for {
		select {
		case <-w.ctx.Done():
			if !ClearCacheWhenClosed || len(w.packetQueue) == 0 {
				return nil
			}
			p, ok = <-w.packetQueue
		case p, ok = <-w.packetQueue:
			if !ClearCacheWhenClosed {
				return nil
			}
		}
		if !ok {
			return nil
		}
		cs.Data = p.Data
		cs.Length = uint32(len(p.Data))
		cs.StreamID = p.StreamID
		cs.Timestamp = p.TimeStamp
		cs.Timestamp += w.BaseTimeStamp()

		if p.IsVideo {
			cs.TypeID = av.TAG_VIDEO
		} else {
			if p.IsMetadata {
				cs.TypeID = av.TAG_SCRIPTDATAAMF0
			} else {
				cs.TypeID = av.TAG_AUDIO
			}
		}

		w.SaveStatics(p.StreamID, uint64(cs.Length), p.IsVideo)
		w.RecTimeStamp(cs.Timestamp, cs.TypeID)
		if err := w.conn.Write(cs); err != nil {
			return err
		}
		Flush.Call(nil)
	}
}

func (v *Writer) Closed() bool {
	select {
	case <-v.ctx.Done():
		return true
	default:
		return false
	}
}

func (v *Writer) Wait() {
	<-v.ctx.Done()
}

func (v *Writer) Dont() <-chan struct{} {
	return v.ctx.Done()
}

func (v *Writer) Close() error {
	if !v.Closed() {
		v.cancel()
	}
	return nil
}
