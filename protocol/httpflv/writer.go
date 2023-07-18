package httpflv

import (
	"context"
	"io"
	"sync"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/container/flv"
)

const (
	headerLen   = 11
	maxQueueNum = 1024
)

type HttpFlvWriter struct {
	flv         *flv.Writer
	packetQueue chan *av.Packet

	closed bool
	lock   *sync.RWMutex
}

func NewFLVWriter(ctx context.Context, w io.Writer, conf ...flv.WriterConf) *HttpFlvWriter {
	writer := &HttpFlvWriter{
		packetQueue: make(chan *av.Packet, maxQueueNum),
		flv:         flv.NewWriter(ctx, w, conf...),
		lock:        new(sync.RWMutex),
	}

	return writer
}

func (w *HttpFlvWriter) Write(p *av.Packet) (err error) {
	w.lock.RLock()
	defer w.lock.RUnlock()

	if w.closed {
		return av.ErrChannelClosed
	}

	select {
	case <-w.flv.Done():
		return w.flv.Err()
	case w.packetQueue <- p:
	default:
		av.DropPacket(w.packetQueue)
	}
	return
}

func (w *HttpFlvWriter) SendPacket(ClearCacheWhenClosed bool) error {
	var p *av.Packet
	var ok bool
	for {
		if ClearCacheWhenClosed {
			p, ok = <-w.packetQueue
		} else {
			select {
			case <-w.flv.Done():
				return nil
			case p, ok = <-w.packetQueue:
			}
		}
		if !ok {
			return nil
		}
		if err := w.flv.Write(p); err != nil {
			return err
		}
	}
}

func (w *HttpFlvWriter) Wait() {
	w.flv.Wait()
}

func (w *HttpFlvWriter) Dont() <-chan struct{} {
	return w.flv.Done()
}

func (w *HttpFlvWriter) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if !w.flv.Closed() {
		w.flv.Close()
		close(w.packetQueue)
		w.closed = true
	}
	return nil
}

func (w *HttpFlvWriter) Closed() bool {
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.closed
}
