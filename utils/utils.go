package utils

import "github.com/zijiren233/livelib/av"

type Timestamp struct {
	videoTimestamp     uint32
	audioTimestamp     uint32
	lastVideoTimestamp uint32
	lastAudioTimestamp uint32
}

func (rw *Timestamp) timeStamp() uint32 {
	if rw.audioTimestamp > rw.videoTimestamp {
		return rw.audioTimestamp
	} else {
		return rw.videoTimestamp
	}
}

func (rw *Timestamp) RecTimeStamp(timestamp, typeID uint32) uint32 {
	if typeID == av.TAG_VIDEO {
		if timestamp < rw.videoTimestamp {
			if rw.lastVideoTimestamp > timestamp {
				rw.videoTimestamp += timestamp
			} else {
				rw.videoTimestamp += timestamp - rw.lastVideoTimestamp
			}
		} else {
			rw.videoTimestamp = timestamp
		}
		rw.lastVideoTimestamp = timestamp
	} else if typeID == av.TAG_AUDIO {
		if timestamp < rw.audioTimestamp {
			if rw.lastAudioTimestamp > timestamp {
				rw.audioTimestamp += timestamp
			} else {
				rw.audioTimestamp += timestamp - rw.lastAudioTimestamp
			}
		} else {
			rw.audioTimestamp = timestamp
		}
		rw.lastAudioTimestamp = timestamp
	}
	return rw.timeStamp()
}
