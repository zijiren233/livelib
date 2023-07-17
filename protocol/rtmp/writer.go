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
	ret := &Writer{
		conn:        conn,
		RWBaser:     av.NewRWBaser(),
		packetQueue: make(chan *av.Packet, maxQueueNum),
		WriteBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}
	ret.ctx, ret.cancel = context.WithCancel(ctx)

	return ret
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

func (v *Writer) SendPacket() error {
	Flush := reflect.ValueOf(v.conn).MethodByName("Flush")
	var cs = new(core.ChunkStream)
	for {
		select {
		case <-v.ctx.Done():
			return v.ctx.Err()
		case p, ok := <-v.packetQueue:
			if !ok {
				return nil
			}
			cs.Data = p.Data
			cs.Length = uint32(len(p.Data))
			cs.StreamID = p.StreamID
			cs.Timestamp = p.TimeStamp
			cs.Timestamp += v.BaseTimeStamp()

			if p.IsVideo {
				cs.TypeID = av.TAG_VIDEO
			} else {
				if p.IsMetadata {
					cs.TypeID = av.TAG_SCRIPTDATAAMF0
				} else {
					cs.TypeID = av.TAG_AUDIO
				}
			}

			v.SaveStatics(p.StreamID, uint64(cs.Length), p.IsVideo)
			v.RecTimeStamp(cs.Timestamp, cs.TypeID)
			err := v.conn.Write(cs)
			if err != nil {
				return err
			}
			Flush.Call(nil)
		}
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

func (v *Writer) Close() error {
	if !v.Closed() {
		close(v.packetQueue)
		v.cancel()
	}
	return v.ctx.Err()
}
