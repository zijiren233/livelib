package rtmp

import (
	"sync/atomic"
	"time"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/container/flv"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Reader struct {
	*av.RWBaser
	demuxer    *flv.Demuxer
	conn       ChunkReader
	ReadBWInfo StaticsBW

	closed uint32
}

func NewReader(conn ChunkReader) *Reader {
	r := &Reader{
		conn:       conn,
		RWBaser:    av.NewRWBaser(),
		demuxer:    flv.NewDemuxer(),
		ReadBWInfo: StaticsBW{0, 0, 0, 0, 0, 0, 0, 0},
	}
	return r
}

func (v *Reader) SaveStatics(streamid uint32, length uint64, isVideoFlag bool) {
	nowInMS := int64(time.Now().UnixNano() / 1e6)

	v.ReadBWInfo.StreamId = streamid
	if isVideoFlag {
		v.ReadBWInfo.VideoDatainBytes = v.ReadBWInfo.VideoDatainBytes + length
	} else {
		v.ReadBWInfo.AudioDatainBytes = v.ReadBWInfo.AudioDatainBytes + length
	}

	if v.ReadBWInfo.LastTimestamp == 0 {
		v.ReadBWInfo.LastTimestamp = nowInMS
	} else if (nowInMS - v.ReadBWInfo.LastTimestamp) >= SAVE_STATICS_INTERVAL {
		diffTimestamp := (nowInMS - v.ReadBWInfo.LastTimestamp) / 1000

		v.ReadBWInfo.VideoSpeedInBytesperMS = (v.ReadBWInfo.VideoDatainBytes - v.ReadBWInfo.LastVideoDatainBytes) * 8 / uint64(diffTimestamp) / 1000
		v.ReadBWInfo.AudioSpeedInBytesperMS = (v.ReadBWInfo.AudioDatainBytes - v.ReadBWInfo.LastAudioDatainBytes) * 8 / uint64(diffTimestamp) / 1000

		v.ReadBWInfo.LastVideoDatainBytes = v.ReadBWInfo.VideoDatainBytes
		v.ReadBWInfo.LastAudioDatainBytes = v.ReadBWInfo.AudioDatainBytes
		v.ReadBWInfo.LastTimestamp = nowInMS
	}
}

func (v *Reader) Read() (p *av.Packet, err error) {
	if v.Closed() {
		return nil, av.ErrClosed
	}
	var cs *core.ChunkStream
	for {
		cs, err = v.conn.Read()
		if err != nil {
			return nil, err
		}
		if cs.TypeID == av.TAG_AUDIO ||
			cs.TypeID == av.TAG_VIDEO ||
			cs.TypeID == av.TAG_SCRIPTDATAAMF0 ||
			cs.TypeID == av.TAG_SCRIPTDATAAMF3 {
			break
		}
	}
	p = new(av.Packet)
	p.IsAudio = cs.TypeID == av.TAG_AUDIO
	p.IsVideo = cs.TypeID == av.TAG_VIDEO
	p.IsMetadata = cs.TypeID == av.TAG_SCRIPTDATAAMF0 || cs.TypeID == av.TAG_SCRIPTDATAAMF3
	p.StreamID = cs.StreamID
	p.Data = cs.Data
	p.TimeStamp = cs.Timestamp

	v.SaveStatics(p.StreamID, uint64(len(p.Data)), p.IsVideo)
	return p, v.demuxer.DemuxH(p)
}

func (v *Reader) Closed() bool {
	return atomic.LoadUint32(&v.closed) == 1
}

func (v *Reader) Close() error {
	if atomic.CompareAndSwapUint32(&v.closed, 0, 1) {
		return nil
	}
	return av.ErrClosed
}
