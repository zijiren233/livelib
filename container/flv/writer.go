package flv

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/livelib/utils"
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
	t         utils.Timestamp
	headerBuf []byte
	w         *stream.Writer
	inited    bool
	bufSize   int

	closed uint64
	wg     sync.WaitGroup
}

type WriterConf func(*Writer)

func WithWriterBuffer(size int) WriterConf {
	return func(w *Writer) {
		w.bufSize = size
	}
}

func NewWriter(w io.Writer, conf ...WriterConf) *Writer {
	writer := &Writer{
		headerBuf: make([]byte, headerLen),
		bufSize:   1024,
	}

	for _, fc := range conf {
		fc(writer)
	}

	writer.w = stream.NewWriter(w, stream.BigEndian)

	return writer
}

func (w *Writer) Write(p *av.Packet) error {
	if w.Closed() {
		return errors.New("flv writer closed")
	}
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
		p = p.Clone()
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
	timestamp := w.t.RecTimeStamp(p.TimeStamp, uint32(typeID))

	preDataLen := dataLen + headerLen
	timestampExt := timestamp >> 24

	return w.w.
		U8(typeID).
		U24(uint32(dataLen)).
		U24(timestamp).
		U8(uint8(timestampExt)).
		U24(0).
		Bytes(p.Data).
		U32(uint32(preDataLen)).Error()
}
func (w *Writer) Close() error {
	if !atomic.CompareAndSwapUint64(&w.closed, 0, 1) {
		return av.ErrClosed
	}
	w.wg.Wait()
	return nil
}

func (w *Writer) Closed() bool {
	return atomic.LoadUint64(&w.closed) == 1
}
