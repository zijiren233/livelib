package core

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/zijiren233/ksync"
	"github.com/zijiren233/livelib/utils/pool"
	"github.com/zijiren233/stream"
)

const (
	_ = iota
	idSetChunkSize
	idAbortMessage
	idAck
	idUserControlMessages
	idWindowAckSize
	idSetPeerBandwidth
)

type Conn struct {
	net.Conn
	chunkSize           uint32
	remoteChunkSize     uint32
	windowAckSize       uint32
	remoteWindowAckSize uint32
	received            uint32
	ackReceived         uint32
	rw                  *ReadWriter
	pool                *pool.Pool
	lock                *ksync.Kmutex
	chunks              map[uint32]*ChunkStream
}

func NewConn(c net.Conn, bufferSize int) *Conn {
	return &Conn{
		Conn:                c,
		chunkSize:           128,
		remoteChunkSize:     128,
		windowAckSize:       2500000,
		remoteWindowAckSize: 2500000,
		pool:                pool.NewPool(),
		rw:                  NewReadWriter(c, bufferSize),
		lock:                ksync.NewKmutex(),
		chunks:              make(map[uint32]*ChunkStream),
	}
}

func (conn *Conn) Read() (c *ChunkStream, err error) {
	for {
		c, err = conn.readNextChunk()
		if err != nil {
			return nil, err
		}
		if c.full() {
			break
		}
	}

	conn.handleControlMsg(c)

	conn.ack(c.Length)

	return
}

func (conn *Conn) readNextChunk() (chunkStream *ChunkStream, err error) {
	h, err := conn.rw.ReadUintBE(1)
	if err != nil {
		return nil, err
	}
	format := h >> 6
	csid := h & 0x3f
	conn.lock.Lock(csid)
	defer conn.lock.Unlock(csid)
	chunkStream, ok := conn.chunks[csid]
	if !ok {
		chunkStream = &ChunkStream{CSID: csid}
		conn.chunks[csid] = chunkStream
	}
	chunkStream.tmpFromat = format
	if chunkStream.remain != 0 && chunkStream.tmpFromat != 3 {
		return nil, fmt.Errorf("invalid remain = %d", chunkStream.remain)
	}
	switch chunkStream.CSID {
	case 0:
		id, err := conn.rw.ReadUintLE(1)
		if err != nil {
			return chunkStream, err
		}
		chunkStream.CSID = id + 64
	case 1:
		id, err := conn.rw.ReadUintLE(2)
		if err != nil {
			return chunkStream, err
		}
		chunkStream.CSID = id + 64
	}

	switch chunkStream.tmpFromat {
	case 0:
		chunkStream.Format = chunkStream.tmpFromat
		chunkStream.Timestamp, err = conn.rw.ReadUintBE(3)
		if err != nil {
			return chunkStream, err
		}
		chunkStream.Length, err = conn.rw.ReadUintBE(3)
		if err != nil {
			return chunkStream, err
		}
		chunkStream.TypeID, err = conn.rw.ReadUintBE(1)
		if err != nil {
			return chunkStream, err
		}
		chunkStream.StreamID, err = conn.rw.ReadUintLE(4)
		if err != nil {
			return chunkStream, err
		}
		if chunkStream.Timestamp == 0xffffff {
			chunkStream.Timestamp, err = conn.rw.ReadUintBE(4)
			if err != nil {
				return chunkStream, err
			}
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.new(conn.pool)
	case 1:
		chunkStream.Format = chunkStream.tmpFromat
		timeStamp, err := conn.rw.ReadUintBE(3)
		if err != nil {
			return chunkStream, err
		}
		chunkStream.Length, err = conn.rw.ReadUintBE(3)
		if err != nil {
			return chunkStream, err
		}
		chunkStream.TypeID, err = conn.rw.ReadUintBE(1)
		if err != nil {
			return chunkStream, err
		}
		if timeStamp == 0xffffff {
			timeStamp, err = conn.rw.ReadUintBE(4)
			if err != nil {
				return chunkStream, err
			}
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new(conn.pool)
	case 2:
		chunkStream.Format = chunkStream.tmpFromat
		timeStamp, err := conn.rw.ReadUintBE(3)
		if err != nil {
			return chunkStream, err
		}
		if timeStamp == 0xffffff {
			timeStamp, err = conn.rw.ReadUintBE(4)
			if err != nil {
				return chunkStream, err
			}
			chunkStream.exted = true
		} else {
			chunkStream.exted = false
		}
		chunkStream.timeDelta = timeStamp
		chunkStream.Timestamp += timeStamp
		chunkStream.new(conn.pool)
	case 3:
		if chunkStream.remain == 0 {
			switch chunkStream.Format {
			case 0:
				if chunkStream.exted {
					timestamp, err := conn.rw.ReadUintBE(4)
					if err != nil {
						return chunkStream, err
					}
					chunkStream.Timestamp = timestamp
				}
			case 1, 2:
				var timedet uint32
				if chunkStream.exted {
					timedet, err = conn.rw.ReadUintBE(4)
					if err != nil {
						return chunkStream, err
					}
				} else {
					timedet = chunkStream.timeDelta
				}
				chunkStream.Timestamp += timedet
			}
			chunkStream.new(conn.pool)
		} else {
			if chunkStream.exted {
				b, err := conn.rw.Peek(4)
				if err != nil {
					return chunkStream, err
				}
				tmpts := binary.BigEndian.Uint32(b)
				if tmpts == chunkStream.Timestamp {
					conn.rw.Discard(4)
				}
			}
		}
	default:
		return chunkStream, fmt.Errorf("invalid format=%d", chunkStream.Format)
	}
	size := chunkStream.remain
	chunkSize := atomic.LoadUint32(&conn.remoteChunkSize)
	if size > chunkSize {
		size = chunkSize
	}

	buf := chunkStream.Data[chunkStream.index : chunkStream.index+size]
	if n, err := conn.rw.Read(buf); err != nil {
		return chunkStream, err
	} else {
		chunkStream.index += uint32(n)
		chunkStream.remain -= uint32(n)
	}
	if chunkStream.remain == 0 {
		chunkStream.got = true
	}

	return
}

func (conn *Conn) Write(c *ChunkStream) error {
	if c.TypeID == idSetChunkSize {
		atomic.StoreUint32(&conn.chunkSize, binary.BigEndian.Uint32(c.Data))
	}

	return c.writeChunk(conn.rw, int(atomic.LoadUint32(&conn.chunkSize)))
}

func (conn *Conn) Flush() error {
	return conn.rw.Flush()
}

func (conn *Conn) Close() error {
	return conn.Conn.Close()
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.Conn.RemoteAddr()
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.Conn.LocalAddr()
}

func (conn *Conn) SetDeadline(t time.Time) error {
	return conn.Conn.SetDeadline(t)
}

func (conn *Conn) NewAck(size uint32) *ChunkStream {
	return initControlMsg(idAck, 4, size)
}

func (conn *Conn) NewSetChunkSize(size uint32) *ChunkStream {
	return initControlMsg(idSetChunkSize, 4, size)
}

func (conn *Conn) NewWindowAckSize(size uint32) *ChunkStream {
	return initControlMsg(idWindowAckSize, 4, size)
}

func (conn *Conn) NewSetPeerBandwidth(size uint32) *ChunkStream {
	ret := initControlMsg(idSetPeerBandwidth, 5, size)
	ret.Data[4] = 2
	return ret
}

func (conn *Conn) handleControlMsg(c *ChunkStream) {
	if c.TypeID == idSetChunkSize {
		atomic.StoreUint32(&conn.remoteChunkSize, binary.BigEndian.Uint32(c.Data))
	} else if c.TypeID == idWindowAckSize {
		atomic.StoreUint32(&conn.remoteWindowAckSize, binary.BigEndian.Uint32(c.Data))
	}
}

func (conn *Conn) ack(size uint32) {
	atomic.AddUint32(&conn.ackReceived, size)
	current := atomic.AddUint32(&conn.received, size)
	if current >= 0xf0000000 {
		atomic.CompareAndSwapUint32(&conn.ackReceived, current, 0)
	}
	if ackReceived := atomic.LoadUint32(&conn.ackReceived); ackReceived >= atomic.LoadUint32(&conn.remoteWindowAckSize) {
		cs := conn.NewAck(ackReceived)
		cs.writeChunk(conn.rw, int(atomic.LoadUint32(&conn.chunkSize)))
		atomic.CompareAndSwapUint32(&conn.ackReceived, ackReceived, 0)
	}
}

func initControlMsg(id, size, value uint32) *ChunkStream {
	ret := &ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   id,
		StreamID: 0,
		Length:   size,
		Data:     make([]byte, size),
	}
	stream.PutU32BE(ret.Data[:size], value)
	return ret
}

const (
	streamBegin      uint32 = 0
	streamEOF        uint32 = 1
	streamDry        uint32 = 2
	setBufferLen     uint32 = 3
	streamIsRecorded uint32 = 4
	pingRequest      uint32 = 6
	pingResponse     uint32 = 7
)

/*
+------------------------------+-------------------------
|     Event Type ( 2- bytes )  | Event Data
+------------------------------+-------------------------
Pay load for the ‘User Control Message’.
*/
func (conn *Conn) userControlMsg(eventType, buflen uint32) ChunkStream {
	var ret ChunkStream
	buflen += 2
	ret = ChunkStream{
		Format:   0,
		CSID:     2,
		TypeID:   4,
		StreamID: 1,
		Length:   buflen,
		Data:     make([]byte, buflen),
	}
	ret.Data[0] = byte(eventType >> 8 & 0xff)
	ret.Data[1] = byte(eventType & 0xff)
	return ret
}

func (conn *Conn) SetBegin() {
	ret := conn.userControlMsg(streamBegin, 4)
	for i := 0; i < 4; i++ {
		ret.Data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	conn.Write(&ret)
}

func (conn *Conn) SetRecorded() {
	ret := conn.userControlMsg(streamIsRecorded, 4)
	for i := 0; i < 4; i++ {
		ret.Data[2+i] = byte(1 >> uint32((3-i)*8) & 0xff)
	}
	conn.Write(&ret)
}
