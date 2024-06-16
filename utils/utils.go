package utils

type Timestamp struct {
	baseTimestamp uint32
	lastTimestamp uint32
}

func (t *Timestamp) RecTimeStamp(timestamp uint32, reconn bool) uint32 {
	if reconn {
		t.baseTimestamp += t.lastTimestamp
		t.lastTimestamp = timestamp
	}
	if t.lastTimestamp < timestamp {
		t.lastTimestamp = timestamp
	}
	return t.baseTimestamp + timestamp
}
