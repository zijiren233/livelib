package utils

import "github.com/zijiren233/livelib/av"

type Timestamp struct {
	t                  uint32
	lastVideoTimestamp uint32
	lastAudioTimestamp uint32
}

func NewTimestamp() *Timestamp {
	return new(Timestamp)
}

func (rw *Timestamp) BaseTimeStamp() uint32 {
	return rw.t
}

func (rw *Timestamp) CalcBaseTimestamp() {
	if rw.lastAudioTimestamp > rw.lastVideoTimestamp {
		rw.t = rw.lastAudioTimestamp
	} else {
		rw.t = rw.lastVideoTimestamp
	}
}

func (rw *Timestamp) RecTimeStamp(timestamp, typeID uint32) {
	if typeID == av.TAG_VIDEO {
		rw.lastVideoTimestamp = timestamp
	} else if typeID == av.TAG_AUDIO {
		rw.lastAudioTimestamp = timestamp
	}
}
