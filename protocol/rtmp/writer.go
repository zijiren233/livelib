package rtmp

import (
	"context"
	"reflect"
	"sync"
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

	closed bool

	lock *sync.RWMutex
}

func NewWriter(ctx context.Context, conn ChunkWriter) *Writer {
	w := &Writer{
		conn:        conn,
		RWBaser:     av.NewRWBaser(),
		packetQueue: make(chan *av.Packet, maxQueueNum),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
		lock:        new(sync.RWMutex),
	}
	w.ctx, w.cancel = context.WithCancel(ctx)

	return w
}

func (w *Writer) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	nowInMS := int64(time.Now().UnixNano() / 1e6)

	w.WriteBWInfo.StreamId = streamid
	if isVideoFlag {
		w.WriteBWInfo.VideoDatainBytes = w.WriteBWInfo.VideoDatainBytes + length
	} else {
		w.WriteBWInfo.AudioDatainBytes = w.WriteBWInfo.AudioDatainBytes + length
	}

	if w.WriteBWInfo.LastTimestamp == 0 {
		w.WriteBWInfo.LastTimestamp = nowInMS
	} else if (nowInMS - w.WriteBWInfo.LastTimestamp) >= SAVE_STATICS_INTERVAL {
		diffTimestamp := (nowInMS - w.WriteBWInfo.LastTimestamp) / 1000

		w.WriteBWInfo.VideoSpeedInBytesperMS = (w.WriteBWInfo.VideoDatainBytes - w.WriteBWInfo.LastVideoDatainBytes) * 8 / uint64(diffTimestamp) / 1000
		w.WriteBWInfo.AudioSpeedInBytesperMS = (w.WriteBWInfo.AudioDatainBytes - w.WriteBWInfo.LastAudioDatainBytes) * 8 / uint64(diffTimestamp) / 1000

		w.WriteBWInfo.LastVideoDatainBytes = w.WriteBWInfo.VideoDatainBytes
		w.WriteBWInfo.LastAudioDatainBytes = w.WriteBWInfo.AudioDatainBytes
		w.WriteBWInfo.LastTimestamp = nowInMS
	}
}

func (w *Writer) Write(p *av.Packet) (err error) {
	w.lock.RLock()
	defer w.lock.RUnlock()

	if w.closed {
		return av.ErrChannelClosed
	}

	select {
	case <-w.ctx.Done():
		return w.ctx.Err()
	case w.packetQueue <- p:
	default:
		av.DropPacket(w.packetQueue)
	}

	return
}

func (w *Writer) SendPacket(ClearCacheWhenClosed bool) error {
	Flush := reflect.ValueOf(w.conn).MethodByName("Flush")
	var cs = new(core.ChunkStream)
	var p *av.Packet
	var ok bool
	for {
		if ClearCacheWhenClosed {
			p, ok = <-w.packetQueue
		} else {
			select {
			case <-w.ctx.Done():
				return nil
			case p, ok = <-w.packetQueue:
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

func (w *Writer) Closed() bool {
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.closed
}

func (w *Writer) Wait() {
	<-w.ctx.Done()
}

func (w *Writer) Dont() <-chan struct{} {
	return w.ctx.Done()
}

func (w *Writer) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if !w.Closed() {
		w.cancel()
		close(w.packetQueue)
	}
	return nil
}
