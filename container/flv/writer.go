package flv

import (
	"bufio"
	"context"
	"io"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/livelib/utils/pio"
)

var (
	FlvHeader          = []byte{0x46, 0x4c, 0x56, 0x01, 0x05, 0x00, 0x00, 0x00, 0x09}
	FlvFirstPreTagSize = []byte{0x00, 0x00, 0x00, 0x00}
	FlvFirstHeader     = append(FlvHeader, FlvFirstPreTagSize...)
)

const (
	headerLen = 11
)

type Writer struct {
	*av.RWBaser
	ctx       context.Context
	cancel    context.CancelFunc
	headerBuf []byte
	w         *bufio.Writer
	inited    bool
	bufSize   int
}

type WriterConf func(*Writer)

func WithWriterBuffer(size int) WriterConf {
	return func(w *Writer) {
		w.bufSize = size
	}
}

func NewWriter(ctx context.Context, w io.Writer, conf ...WriterConf) *Writer {
	writer := &Writer{
		RWBaser:   av.NewRWBaser(),
		headerBuf: make([]byte, headerLen),
		bufSize:   1024,
	}
	for _, fc := range conf {
		fc(writer)
	}
	writer.w = bufio.NewWriterSize(w, writer.bufSize)
	writer.ctx, writer.cancel = context.WithCancel(ctx)

	return writer
}

func (writer *Writer) Write(p *av.Packet) error {
	select {
	case <-writer.ctx.Done():
		return writer.ctx.Err()
	default:
	}
	if !writer.inited {
		_, err := writer.w.Write(FlvFirstHeader)
		if err != nil {
			return err
		}
		writer.inited = true
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
		return nil
	}
	dataLen := len(p.Data)
	timestamp := p.TimeStamp + writer.BaseTimeStamp()
	writer.RWBaser.RecTimeStamp(timestamp, uint32(typeID))

	preDataLen := dataLen + headerLen
	timestampExt := timestamp >> 24

	pio.PutU8(writer.headerBuf[0:1], uint8(typeID))
	pio.PutU24BE(writer.headerBuf[1:4], uint32(dataLen))
	pio.PutU24BE(writer.headerBuf[4:7], uint32(timestamp))
	pio.PutU8(writer.headerBuf[7:8], uint8(timestampExt))

	if _, err := writer.w.Write(writer.headerBuf); err != nil {
		return err
	}

	if _, err := writer.w.Write(p.Data); err != nil {
		return err
	}

	pio.PutU32BE(writer.headerBuf[:4], uint32(preDataLen))
	if _, err := writer.w.Write(writer.headerBuf[:4]); err != nil {
		return err
	}
	if err := writer.w.Flush(); err != nil {
		return err
	}

	return nil
}

func (writer *Writer) Closed() bool {
	select {
	case <-writer.ctx.Done():
		return true
	default:
		return false
	}
}

func (writer *Writer) Close() error {
	if !writer.Closed() {
		writer.cancel()
	}
	return writer.ctx.Err()
}

func (writer *Writer) Wait() {
	<-writer.ctx.Done()
}

func (writer *Writer) Done() <-chan struct{} {
	return writer.ctx.Done()
}

func (writer *Writer) Err() error {
	return writer.ctx.Err()
}
