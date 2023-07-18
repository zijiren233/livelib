package httpflv

import (
	"context"
	"io"

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
}

func NewFLVWriter(ctx context.Context, w io.Writer, conf ...flv.WriterConf) *HttpFlvWriter {
	writer := &HttpFlvWriter{
		packetQueue: make(chan *av.Packet, maxQueueNum),
		flv:         flv.NewWriter(ctx, w, conf...),
	}

	return writer
}

func (flvWriter *HttpFlvWriter) Write(p *av.Packet) (err error) {
	select {
	case <-flvWriter.flv.Done():
		return flvWriter.flv.Err()
	case flvWriter.packetQueue <- p:
	default:
		av.DropPacket(flvWriter.packetQueue)
	}
	return
}

func (w *HttpFlvWriter) SendPacket(ClearCacheWhenClosed bool) error {
	var p *av.Packet
	var ok bool
	for {
		select {
		case <-w.flv.Done():
			if !ClearCacheWhenClosed || len(w.packetQueue) == 0 {
				return nil
			}
			p, ok = <-w.packetQueue
		case p, ok = <-w.packetQueue:
			if !ClearCacheWhenClosed {
				return nil
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
	if !w.flv.Closed() {
		w.flv.Close()
	}
	return nil
}

func (w *HttpFlvWriter) Closed() bool {
	return w.flv.Closed()
}
