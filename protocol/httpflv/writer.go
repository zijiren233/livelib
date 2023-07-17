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

func (flvWriter *HttpFlvWriter) SendPacket() error {
	for {
		select {
		case <-flvWriter.flv.Done():
			return flvWriter.flv.Err()
		case p := <-flvWriter.packetQueue:
			if err := flvWriter.flv.Write(p); err != nil {
				return err
			}
		}
	}
}

func (flvWriter *HttpFlvWriter) Wait() {
	flvWriter.flv.Wait()
}

func (flvWriter *HttpFlvWriter) Dont() <-chan struct{} {
	return flvWriter.flv.Done()
}

func (flvWriter *HttpFlvWriter) Close() error {
	return flvWriter.flv.Close()
}

func (flvWriter *HttpFlvWriter) Closed() bool {
	return flvWriter.flv.Closed()
}
