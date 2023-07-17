package flv

import (
	"bufio"
	"bytes"
	"errors"
	"io"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/livelib/utils/pio"
)

type Reader struct {
	r            *bufio.Reader
	inited       bool
	demuxer      *Demuxer
	tagHeaderBuf []byte
	bufSize      int
}

type ReaderConf func(*Reader)

func WithReaderBuffer(size int) ReaderConf {
	return func(r *Reader) {
		r.bufSize = size
	}
}

func NewReader(r io.Reader, conf ...ReaderConf) *Reader {
	reader := &Reader{
		tagHeaderBuf: make([]byte, headerLen),
		demuxer:      NewDemuxer(),
		bufSize:      1024,
	}
	for _, rc := range conf {
		rc(reader)
	}
	reader.r = bufio.NewReaderSize(r, reader.bufSize)
	return reader
}

var ErrHeader = errors.New("read flv header error")
var ErrPreDataLen = errors.New("read flv pre data len error")

func (fr *Reader) Read() (p *av.Packet, err error) {
	if !fr.inited {
		if _, err := io.ReadFull(fr.r, fr.tagHeaderBuf[:9]); err != nil {
			return nil, err
		} else if !bytes.Equal(fr.tagHeaderBuf[:9], FlvHeader) {
			return nil, ErrHeader
		} else if _, err := io.ReadFull(fr.r, fr.tagHeaderBuf[:4]); err != nil {
			return nil, err
		} else if !bytes.Equal(fr.tagHeaderBuf[:4], FlvFirstPreTagSize) {
			return nil, ErrHeader
		}
		fr.inited = true
	}

	if _, err := io.ReadFull(fr.r, fr.tagHeaderBuf); err != nil {
		return nil, err
	}
	p = new(av.Packet)
	p.IsVideo = fr.tagHeaderBuf[0] == av.TAG_VIDEO
	p.IsAudio = fr.tagHeaderBuf[0] == av.TAG_AUDIO
	p.IsMetadata = fr.tagHeaderBuf[0] == av.TAG_SCRIPTDATAAMF0

	dataLen := pio.U24BE(fr.tagHeaderBuf[1:4])
	timestampbase := pio.U24BE(fr.tagHeaderBuf[4:7])
	timestampExt := pio.U8(fr.tagHeaderBuf[7:8])

	p.TimeStamp = uint32(timestampExt)<<24 | timestampbase
	p.Data = make([]byte, dataLen)
	if _, err := io.ReadFull(fr.r, p.Data); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(fr.r, fr.tagHeaderBuf[:4]); err != nil {
		return nil, err
	}
	preDataLen := pio.U32BE(fr.tagHeaderBuf[:4])
	if uint32(preDataLen) != dataLen+headerLen {
		return nil, ErrPreDataLen
	}

	if p.IsMetadata {
		p.Data, err = amf.MetaDataReform(p.Data, amf.ADD)
		if err != nil {
			return
		}
	}

	return p, nil
}
