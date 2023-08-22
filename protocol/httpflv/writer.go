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
	*av.RWBaser
	headerBuf []byte
	w         *stream.Writer
	inited    bool
	bufSize   int

	packetQueue chan *av.Packet
	ctx         context.Context
	cancel      context.CancelFunc

	lock *sync.RWMutex
}

type HttpFlvWriterConf func(*HttpFlvWriter)

func WithWriterBuffer(size int) HttpFlvWriterConf {
	return func(w *HttpFlvWriter) {
		w.bufSize = size
	}
}

func NewHttpFLVWriter(ctx context.Context, w io.Writer, conf ...HttpFlvWriterConf) *HttpFlvWriter {
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

	writer.ctx, writer.cancel = context.WithCancel(ctx)
	writer.w = stream.NewWriter(w, stream.WithWriterBuffer(true))

	return writer
}

func (w *HttpFlvWriter) Write(p *av.Packet) (err error) {
	w.lock.RLock()

	if w.closed() {
		w.lock.RUnlock()
		return av.ErrChannelClosed
	}

	select {
	case <-w.ctx.Done():
		w.lock.RUnlock()
		return w.ctx.Err()
	case w.packetQueue <- p:
		w.lock.RUnlock()
	default:
		w.lock.RUnlock()
		w.lock.Lock()
		av.DropPacket(w.packetQueue)
		w.lock.Unlock()
	}
	return
}

func (w *HttpFlvWriter) SendPacket(ClearCacheWhenClosed bool) error {
	var p *av.Packet
	var ok bool
	for {
		w.lock.RLock()
		if w.closed() {
			if len(w.packetQueue) == 0 {
				w.lock.RUnlock()
				return nil
			}
			p, ok = <-w.packetQueue
			w.lock.RUnlock()
		} else {
			w.lock.RUnlock()
			select {
			case <-w.ctx.Done():
				if ClearCacheWhenClosed {
					continue
				}
				return nil
			case p, ok = <-w.packetQueue:
			}
		}
		if !ok {
			return nil
		}
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
			U24BE(uint32(dataLen)).
			U24BE(uint32(timestamp)).
			U8(uint8(timestampExt)).
			U24BE(0).
			Bytes(p.Data).
			U32BE(uint32(preDataLen)).Error(); err != nil {
			return err
		}
	}
}

func (w *HttpFlvWriter) Wait() {
	<-w.ctx.Done()
}

func (w *HttpFlvWriter) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed() {
		return w.ctx.Err()
	}
	w.cancel()
	close(w.packetQueue)
	return nil
}

func (w *HttpFlvWriter) Closed() bool {
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.closed()
}

func (w *HttpFlvWriter) closed() bool {
	select {
	case <-w.ctx.Done():
		return true
	default:
		return false
	}
}
