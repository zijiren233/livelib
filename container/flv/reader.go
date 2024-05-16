package flv

import (
	"bytes"
	"errors"
	"io"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
	"github.com/zijiren233/stream"
)

type Reader struct {
	r            *stream.Reader
	inited       bool
	demuxer      *Demuxer
	tagHeaderBuf []byte
	bufSize      int
	FlvTagHeader FlvTagHeader
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
	reader.r = stream.NewReader(r, stream.BigEndian)
	return reader
}

var ErrHeader = errors.New("read flv header error")
var ErrPreDataLen = errors.New("read flv pre data len error")

func (fr *Reader) Read() (p *av.Packet, err error) {
	if !fr.inited {
		if err := fr.r.Bytes(fr.tagHeaderBuf[:9]).Error(); err != nil {
			return nil, err
		} else if !bytes.Equal(fr.tagHeaderBuf[:9], FlvHeader) {
			return nil, ErrHeader
		} else if err := fr.r.Bytes(fr.tagHeaderBuf[:4]).Error(); err != nil {
			return nil, err
		} else if !bytes.Equal(fr.tagHeaderBuf[:4], FlvFirstPreTagSize) {
			return nil, ErrHeader
		}
		fr.inited = true
	}
	p = new(av.Packet)

	if err := fr.r.
		U8(&fr.FlvTagHeader.TagType).
		U24(&fr.FlvTagHeader.DataSize).
		U24(&fr.FlvTagHeader.Timestamp).
		U8(&fr.FlvTagHeader.TimestampExtended).
		U24(&fr.FlvTagHeader.StreamID).
		Error(); err != nil {
		return nil, err
	}
	p.IsVideo = fr.FlvTagHeader.TagType == av.TAG_VIDEO
	p.IsAudio = fr.FlvTagHeader.TagType == av.TAG_AUDIO
	p.IsMetadata = fr.FlvTagHeader.TagType == av.TAG_SCRIPTDATAAMF0

	p.TimeStamp = uint32(fr.FlvTagHeader.TimestampExtended)<<24 | fr.FlvTagHeader.Timestamp
	p.Data = make([]byte, fr.FlvTagHeader.DataSize)
	if err := fr.r.
		Bytes(p.Data).
		U32(&fr.FlvTagHeader.PreTagSzie).
		Error(); err != nil {
		return nil, err
	}
	if fr.FlvTagHeader.PreTagSzie != fr.FlvTagHeader.DataSize+headerLen {
		return nil, ErrPreDataLen
	}

	if p.IsMetadata {
		p.Data, err = amf.MetaDataReform(p.Data, amf.ADD)
		if err != nil {
			return
		}
	}

	return p, fr.demuxer.DemuxH(p)
}
