package httpflv

import (
	"bufio"
	"context"
	"errors"
	"io"
	"sync"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/container/flv"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/livelib/utils/pio"
)

const (
	headerLen   = 11
	maxQueueNum = 1024
)

type HttpFlvWriter struct {
	*av.RWBaser
	headerBuf []byte
	w         *bufio.Writer
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
	writer.w = bufio.NewWriterSize(w, writer.bufSize)

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
		if ClearCacheWhenClosed {
			w.lock.RLock()
			if w.closed() && len(w.packetQueue) == 0 {
				w.lock.RUnlock()
				return nil
			}
			p, ok = <-w.packetQueue
			w.lock.RUnlock()
		} else {
			select {
			case <-w.ctx.Done():
				return nil
			case p, ok = <-w.packetQueue:
			}
		}
		if !ok {
			return nil
		}
		if !w.inited {
			_, err := w.w.Write(flv.FlvFirstHeader)
			if err != nil {
				return err
			}
			w.inited = true
		}

		var typeID int

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

		pio.PutU8(w.headerBuf[0:1], uint8(typeID))
		pio.PutU24BE(w.headerBuf[1:4], uint32(dataLen))
		pio.PutU24BE(w.headerBuf[4:7], uint32(timestamp))
		pio.PutU8(w.headerBuf[7:8], uint8(timestampExt))

		if _, err := w.w.Write(w.headerBuf); err != nil {
			return err
		}

		if _, err := w.w.Write(p.Data); err != nil {
			return err
		}

		pio.PutU32BE(w.headerBuf[:4], uint32(preDataLen))
		if _, err := w.w.Write(w.headerBuf[:4]); err != nil {
			return err
		}
		if err := w.w.Flush(); err != nil {
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
