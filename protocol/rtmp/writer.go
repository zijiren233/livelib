package rtmp

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
	"github.com/zijiren233/livelib/utils"
)

type Writer struct {
	t           utils.Timestamp
	conn        ChunkWriter
	packetQueue chan *av.Packet
	WriteBWInfo StaticsBW

	closed uint64
	wg     sync.WaitGroup
}

func NewWriter(conn ChunkWriter) *Writer {
	w := &Writer{
		conn:        conn,
		packetQueue: make(chan *av.Packet, maxQueueNum),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}

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
	w.wg.Add(1)
	defer w.wg.Done()

	if w.Closed() {
		return av.ErrClosed
	}

	p = p.Clone()
	p.TimeStamp = w.t.RecTimeStamp(p.TimeStamp)

	select {
	case w.packetQueue <- p:
	default:
		av.DropPacket(w.packetQueue)
	}
	return
}

func (w *Writer) SendPacket() error {
	Flush := reflect.ValueOf(w.conn).MethodByName("Flush")
	var cs = new(core.ChunkStream)
	for p := range w.packetQueue {
		cs.Data = p.Data
		cs.Length = uint32(len(p.Data))
		cs.StreamID = p.StreamID

		cs.TypeID = uint32(p.Type())

		w.SaveStatics(p.StreamID, uint64(cs.Length), p.IsVideo)
		cs.Timestamp = p.TimeStamp
		if err := w.conn.Write(cs); err != nil {
			return err
		}
		v := Flush.Call(nil)
		if v[0].Interface() != nil {
			return v[0].Interface().(error)
		}
	}
	return nil
}

func (w *Writer) Close() error {
	if !atomic.CompareAndSwapUint64(&w.closed, 0, 1) {
		return av.ErrClosed
	}
	w.wg.Wait()
	close(w.packetQueue)
	return nil
}

func (w *Writer) Closed() bool {
	return atomic.LoadUint64(&w.closed) == 1
}
