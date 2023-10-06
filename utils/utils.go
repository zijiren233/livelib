package utils

type Timestamp struct {
	baseTimestamp uint32
	lastTimestamp uint32
}

func (t *Timestamp) RecTimeStamp(timestamp uint32) uint32 {
	// if typeID == av.TAG_VIDEO {
	// 	if timestamp < rw.videoTimestamp {
	// 		if rw.lastVideoTimestamp > timestamp {
	// 			rw.videoTimestamp += timestamp
	// 		} else {
	// 			rw.videoTimestamp += timestamp - rw.lastVideoTimestamp
	// 		}
	// 	} else {
	// 		rw.videoTimestamp = timestamp
	// 	}
	// 	rw.lastVideoTimestamp = timestamp
	// } else if typeID == av.TAG_AUDIO {
	// 	if timestamp < rw.audioTimestamp {
	// 		if rw.lastAudioTimestamp > timestamp {
	// 			rw.audioTimestamp += timestamp
	// 		} else {
	// 			rw.audioTimestamp += timestamp - rw.lastAudioTimestamp
	// 		}
	// 	} else {
	// 		rw.audioTimestamp = timestamp
	// 	}
	// 	rw.lastAudioTimestamp = timestamp
	// }
	// return rw.timeStamp()

	if t.lastTimestamp > timestamp+100 {
		t.baseTimestamp += t.lastTimestamp
		t.lastTimestamp = timestamp
	}
	if t.lastTimestamp < timestamp {
		t.lastTimestamp = timestamp
	}
	return t.baseTimestamp + timestamp
}
