package flv

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/stream"
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
	headerBuf []byte
	w         *stream.Writer
	inited    bool
	bufSize   int

	ctx    context.Context
	cancel context.CancelFunc
	lock   *sync.RWMutex
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
		lock:      new(sync.RWMutex),
	}

	for _, fc := range conf {
		fc(writer)
	}

	writer.ctx, writer.cancel = context.WithCancel(ctx)
	writer.w = stream.NewWriter(w, stream.BigEndian)

	return writer
}

func (w *Writer) Write(p *av.Packet) error {
	w.lock.RLock()
	if w.closed() {
		w.lock.RUnlock()
		return errors.New("flv writer closed")
	}
	w.lock.RUnlock()
	if !w.inited {
		if err := w.w.Bytes(FlvFirstHeader).Error(); err != nil {
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
		return nil
	}
	dataLen := len(p.Data)
	timestamp := p.TimeStamp + w.BaseTimeStamp()
	w.RWBaser.RecTimeStamp(timestamp, uint32(typeID))

	preDataLen := dataLen + headerLen
	timestampExt := timestamp >> 24

	return w.w.
		U8(typeID).
		U24(uint32(dataLen)).
		U24(uint32(timestamp)).
		U8(uint8(timestampExt)).
		U24(0).
		Bytes(p.Data).
		U32(uint32(preDataLen)).Error()
}

func (w *Writer) Closed() bool {
	w.lock.RLock()
	defer w.lock.RUnlock()
	return w.closed()
}

func (w *Writer) closed() bool {
	select {
	case <-w.ctx.Done():
		return true
	default:
		return false
	}
}

func (w *Writer) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed() {
		return errors.New("Closed")
	}
	w.cancel()
	return nil
}

func (w *Writer) Wait() {
	<-w.ctx.Done()
}
