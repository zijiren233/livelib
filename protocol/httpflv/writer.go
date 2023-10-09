package httpflv

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/container/flv"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/livelib/utils"
	"github.com/zijiren233/stream"
)

const (
	headerLen   = 11
	maxQueueNum = 1024
)

type HttpFlvWriter struct {
	t         utils.Timestamp
	headerBuf []byte
	w         *stream.Writer
	inited    bool
	bufSize   int

	packetQueue chan *av.Packet

	closed uint32
	wg     sync.WaitGroup
}

type HttpFlvWriterConf func(*HttpFlvWriter)

func WithWriterBuffer(size int) HttpFlvWriterConf {
	return func(w *HttpFlvWriter) {
		w.bufSize = size
	}
}

func NewHttpFLVWriter(w io.Writer, conf ...HttpFlvWriterConf) *HttpFlvWriter {
	writer := &HttpFlvWriter{
		headerBuf:   make([]byte, headerLen),
		bufSize:     1024,
		packetQueue: make(chan *av.Packet, maxQueueNum),
	}

	for _, hfwc := range conf {
		hfwc(writer)
	}

	writer.w = stream.NewWriter(w, stream.BigEndian)

	return writer
}

func (w *HttpFlvWriter) Write(p *av.Packet) (err error) {
	w.wg.Add(1)
	defer w.wg.Done()

	if w.Closed() {
		return av.ErrClosed
	}
	p = p.Clone()
	p.TimeStamp = w.t.RecTimeStamp(p.TimeStamp)
	select {
	case w.packetQueue <- p:
	default:
		av.DropPacket(w.packetQueue)
	}
	return
}

func (w *HttpFlvWriter) SendPacket() error {
	var typeID uint8
	for p := range w.packetQueue {
		if !w.inited {
			if err := w.w.Bytes(flv.FlvFirstHeader).Error(); err != nil {
				return err
			}
			w.inited = true
		}

		if p.IsVideo {
			typeID = av.TAG_VIDEO
		} else if p.IsMetadata {
			var err error
			typeID = av.TAG_SCRIPTDATAAMF0
			p = p.DeepClone()
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
		preDataLen := dataLen + headerLen
		timestampExt := p.TimeStamp >> 24

		if err := w.w.
			U8(typeID).
			U24(uint32(dataLen)).
			U24(p.TimeStamp).
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
	if !atomic.CompareAndSwapUint32(&w.closed, 0, 1) {
		return av.ErrClosed
	}
	w.wg.Wait()
	close(w.packetQueue)
	return nil
}

func (w *HttpFlvWriter) Closed() bool {
	return atomic.LoadUint32(&w.closed) == 1
}
