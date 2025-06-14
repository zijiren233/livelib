package core

import (
	"fmt"

	"github.com/zijiren233/livelib/av"
)

type ChunkStream struct {
	Format    uint32
	CSID      uint32
	Timestamp uint32
	Length    uint32
	TypeID    uint32
	StreamID  uint32
	timeDelta uint32
	exted     bool
	index     uint32
	remain    uint32
	got       bool
	tmpFromat uint32
	Data      []byte
}

func (chunkStream *ChunkStream) full() bool {
	return chunkStream.got
}

func (chunkStream *ChunkStream) init() {
	chunkStream.got = false
	chunkStream.index = 0
	chunkStream.remain = chunkStream.Length
	chunkStream.Data = make([]byte, chunkStream.Length)
}

func (chunkStream *ChunkStream) writeHeader(w *ReadWriter) error {
	// Chunk Basic Header
	h := chunkStream.Format << 6
	switch {
	case chunkStream.CSID < 64:
		h |= chunkStream.CSID
		if err := w.WriteUintBE(h, 1); err != nil {
			return err
		}
	case chunkStream.CSID-64 < 256:
		h |= 0
		if err := w.WriteUintBE(h, 1); err != nil {
			return err
		}
		if err := w.WriteUintLE(chunkStream.CSID-64, 1); err != nil {
			return err
		}
	case chunkStream.CSID-64 < 65536:
		h |= 1
		w.WriteUintBE(h, 1)
		if err := w.WriteUintBE(h, 1); err != nil {
			return err
		}
		if err := w.WriteUintLE(chunkStream.CSID-64, 2); err != nil {
			return err
		}
	}
	// Chunk Message Header
	ts := chunkStream.Timestamp
	if chunkStream.Format == 3 {
		goto END
	}
	if chunkStream.Timestamp > 0xffffff {
		ts = 0xffffff
	}
	if err := w.WriteUintBE(ts, 3); err != nil {
		return err
	}
	if chunkStream.Format == 2 {
		goto END
	}
	if chunkStream.Length > 0xffffff {
		return fmt.Errorf("length=%d", chunkStream.Length)
	}
	if err := w.WriteUintBE(chunkStream.Length, 3); err != nil {
		return err
	}
	if err := w.WriteUintBE(chunkStream.TypeID, 1); err != nil {
		return err
	}
	if chunkStream.Format == 1 {
		goto END
	}
	if err := w.WriteUintLE(chunkStream.StreamID, 4); err != nil {
		return err
	}
END:
	// Extended Timestamp
	if ts >= 0xffffff {
		if err := w.WriteUintBE(chunkStream.Timestamp, 4); err != nil {
			return err
		}
	}
	return nil
}

func (chunkStream *ChunkStream) writeChunk(w *ReadWriter, chunkSize uint32) error {
	switch chunkStream.TypeID {
	case av.TAG_AUDIO:
		chunkStream.CSID = 4
	case av.TAG_VIDEO, av.TAG_SCRIPTDATAAMF0, av.TAG_SCRIPTDATAAMF3:
		chunkStream.CSID = 6
	}

	totalLen := uint32(0)
	numChunks := (chunkStream.Length / chunkSize)
	for i := uint32(0); i <= numChunks; i++ {
		if totalLen == chunkStream.Length {
			break
		}
		if i == 0 {
			chunkStream.Format = uint32(0)
		} else {
			chunkStream.Format = uint32(3)
		}
		if err := chunkStream.writeHeader(w); err != nil {
			return err
		}
		inc := chunkSize
		start := i * chunkSize
		if uint32(len(chunkStream.Data))-start <= inc {
			inc = uint32(len(chunkStream.Data)) - start
		}
		totalLen += inc
		end := start + inc
		if _, err := w.Write(chunkStream.Data[start:end]); err != nil {
			return err
		}
	}

	return nil
}

// func (chunkStream *ChunkStream) readChunk(r *ReadWriter, chunkSize uint32, pool *pool.Pool) (err error) {
// 	if chunkStream.remain != 0 && chunkStream.tmpFromat != 3 {
// 		return fmt.Errorf("invalid remain = %d", chunkStream.remain)
// 	}
// 	switch chunkStream.CSID {
// 	case 0:
// 		id, err := r.ReadUintLE(1)
// 		if err != nil {
// 			return err
// 		}
// 		chunkStream.CSID = id + 64
// 	case 1:
// 		id, err := r.ReadUintLE(2)
// 		if err != nil {
// 			return err
// 		}
// 		chunkStream.CSID = id + 64
// 	}

// 	switch chunkStream.tmpFromat {
// 	case 0:
// 		chunkStream.Format = chunkStream.tmpFromat
// 		chunkStream.Timestamp, err = r.ReadUintBE(3)
// 		if err != nil {
// 			return err
// 		}
// 		chunkStream.Length, err = r.ReadUintBE(3)
// 		if err != nil {
// 			return err
// 		}
// 		chunkStream.TypeID, err = r.ReadUintBE(1)
// 		if err != nil {
// 			return err
// 		}
// 		chunkStream.StreamID, err = r.ReadUintLE(4)
// 		if err != nil {
// 			return err
// 		}
// 		if chunkStream.Timestamp == 0xffffff {
// 			chunkStream.Timestamp, err = r.ReadUintBE(4)
// 			if err != nil {
// 				return err
// 			}
// 			chunkStream.exted = true
// 		} else {
// 			chunkStream.exted = false
// 		}
// 		chunkStream.new(pool)
// 	case 1:
// 		chunkStream.Format = chunkStream.tmpFromat
// 		timeStamp, err := r.ReadUintBE(3)
// 		if err != nil {
// 			return err
// 		}
// 		chunkStream.Length, err = r.ReadUintBE(3)
// 		if err != nil {
// 			return err
// 		}
// 		chunkStream.TypeID, err = r.ReadUintBE(1)
// 		if err != nil {
// 			return err
// 		}
// 		if timeStamp == 0xffffff {
// 			timeStamp, err = r.ReadUintBE(4)
// 			if err != nil {
// 				return err
// 			}
// 			chunkStream.exted = true
// 		} else {
// 			chunkStream.exted = false
// 		}
// 		chunkStream.timeDelta = timeStamp
// 		chunkStream.Timestamp += timeStamp
// 		chunkStream.new(pool)
// 	case 2:
// 		chunkStream.Format = chunkStream.tmpFromat
// 		timeStamp, err := r.ReadUintBE(3)
// 		if err != nil {
// 			return err
// 		}
// 		if timeStamp == 0xffffff {
// 			timeStamp, err = r.ReadUintBE(4)
// 			if err != nil {
// 				return err
// 			}
// 			chunkStream.exted = true
// 		} else {
// 			chunkStream.exted = false
// 		}
// 		chunkStream.timeDelta = timeStamp
// 		chunkStream.Timestamp += timeStamp
// 		chunkStream.new(pool)
// 	case 3:
// 		if chunkStream.remain == 0 {
// 			switch chunkStream.Format {
// 			case 0:
// 				if chunkStream.exted {
// 					timestamp, err := r.ReadUintBE(4)
// 					if err != nil {
// 						return err
// 					}
// 					chunkStream.Timestamp = timestamp
// 				}
// 			case 1, 2:
// 				var timedet uint32
// 				if chunkStream.exted {
// 					timedet, err = r.ReadUintBE(4)
// 					if err != nil {
// 						return err
// 					}
// 				} else {
// 					timedet = chunkStream.timeDelta
// 				}
// 				chunkStream.Timestamp += timedet
// 			}
// 			chunkStream.new(pool)
// 		} else {
// 			if chunkStream.exted {
// 				b, err := r.Peek(4)
// 				if err != nil {
// 					return err
// 				}
// 				tmpts := binary.BigEndian.Uint32(b)
// 				if tmpts == chunkStream.Timestamp {
// 					r.Discard(4)
// 				}
// 			}
// 		}
// 	default:
// 		return fmt.Errorf("invalid format=%d", chunkStream.Format)
// 	}
// 	size := int(chunkStream.remain)
// 	if size > int(chunkSize) {
// 		size = int(chunkSize)
// 	}

// 	buf := chunkStream.Data[chunkStream.index : chunkStream.index+uint32(size)]
// 	if _, err := r.Read(buf); err != nil {
// 		return err
// 	}
// 	chunkStream.index += uint32(size)
// 	chunkStream.remain -= uint32(size)
// 	if chunkStream.remain == 0 {
// 		chunkStream.got = true
// 	}

// 	return nil
// }
