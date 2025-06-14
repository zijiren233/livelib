package httpflv

import (
	"context"
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
	headerBuf []byte
	w         *stream.Writer
	inited    bool
	bufSize   int

	packetQueue chan *av.Packet

	closed bool
	mu     sync.RWMutex
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
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return av.ErrClosed
	}

	for {
		select {
		case w.packetQueue <- p:
			return
		default:
			av.DropPacket(w.packetQueue)
		}
	}
}

func (w *HttpFlvWriter) SendPacket(ctx context.Context) error {
	var typeID uint8
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-w.packetQueue:
			if !ok {
				return nil
			}
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
	}
}

func (w *HttpFlvWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return av.ErrClosed
	}
	w.closed = true
	close(w.packetQueue)
	return nil
}
