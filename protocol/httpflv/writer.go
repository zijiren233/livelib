package httpflv

import (
	"errors"
	"io"
	"sync"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/container/flv"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/stream"
)

const (
	headerLen   = 11
	maxQueueNum = 1024
)

type HttpFlvWriter struct {
	*av.RWBaser
	headerBuf []byte
	w         *stream.Writer
	inited    bool
	bufSize   int

	packetQueue chan *av.Packet

	closed bool
	lock   *sync.RWMutex
}

type HttpFlvWriterConf func(*HttpFlvWriter)

func WithWriterBuffer(size int) HttpFlvWriterConf {
	return func(w *HttpFlvWriter) {
		w.bufSize = size
	}
}

func NewHttpFLVWriter(w io.Writer, conf ...HttpFlvWriterConf) *HttpFlvWriter {
	writer := &HttpFlvWriter{
		RWBaser:     av.NewRWBaser(),
		headerBuf:   make([]byte, headerLen),
		bufSize:     1024,
		packetQueue: make(chan *av.Packet, maxQueueNum),
		lock:        new(sync.RWMutex),
	}

	for _, hfwc := range conf {
		hfwc(writer)
	}

	writer.w = stream.NewWriter(w, stream.BigEndian)

	return writer
}

func (w *HttpFlvWriter) Write(p *av.Packet) (err error) {
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

func (w *HttpFlvWriter) SendPacket() error {
	for p := range w.packetQueue {
		if !w.inited {
			if err := w.w.Bytes(flv.FlvFirstHeader).Error(); err != nil {
				return err
			}
			w.inited = true
		}

		var typeID uint8

		if p.IsVideo {
			typeID = av.TAG_VIDEO
		} else if p.IsMetadata {
			var err error
			typeID = av.TAG_SCRIPTDATAAMF0
			p = p.NewPacketData()
			p.Data, err = amf.MetaDataReform(p.Data, amf.DEL)
			if err != nil {
				return err
			}
		} else if p.IsAudio {
			typeID = av.TAG_AUDIO
		} else {
			return errors.New("not allowed packet type")
		}
		dataLen := len(p.Data)
		timestamp := p.TimeStamp + w.BaseTimeStamp()
		w.RWBaser.RecTimeStamp(timestamp, uint32(typeID))

		preDataLen := dataLen + headerLen
		timestampExt := timestamp >> 24

		if err := w.w.
			U8(typeID).
			U24(uint32(dataLen)).
			U24(uint32(timestamp)).
			U8(uint8(timestampExt)).
			U24(0).
			Bytes(p.Data).
			U32(uint32(preDataLen)).Error(); err != nil {
			return err
		}
	}
	return nil
}

func (w *HttpFlvWriter) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed {
		return av.ErrClosed
	}
	w.closed = true
	close(w.packetQueue)
	return nil
}

func (w *HttpFlvWriter) Closed() bool {
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.closed
}
