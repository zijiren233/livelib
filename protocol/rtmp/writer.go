package rtmp

import (
	"reflect"
	"sync"
	"time"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Writer struct {
	*av.RWBaser
	conn        ChunkWriter
	packetQueue chan *av.Packet
	WriteBWInfo StaticsBW

	closed bool
	lock   *sync.RWMutex
}

func NewWriter(conn ChunkWriter) *Writer {
	w := &Writer{
		conn:        conn,
		RWBaser:     av.NewRWBaser(),
		packetQueue: make(chan *av.Packet, maxQueueNum),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
		lock:        new(sync.RWMutex),
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
	w.lock.RLock()
	defer w.lock.RUnlock()

	if w.closed {
		return av.ErrClosed
	}

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
		v := Flush.Call(nil)
		if v[0].Interface() != nil {
			return v[0].Interface().(error)
		}
	}
	return nil
}

func (w *Writer) Closed() bool {
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.closed
}

func (w *Writer) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	close(w.packetQueue)
	return nil
}
