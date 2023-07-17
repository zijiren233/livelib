package flv

import (
	"bufio"
	"bytes"
	"errors"
	"io"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/utils/pio"
)

type FlvReader struct {
	r            *bufio.Reader
	inited       bool
	demuxer      *Demuxer
	tagHeaderBuf []byte
}

func NewFlvReader(r io.Reader) *FlvReader {
	return &FlvReader{r: bufio.NewReader(r), tagHeaderBuf: make([]byte, headerLen), demuxer: NewDemuxer()}
}

var ErrHeader = errors.New("read flv header error")
var ErrPreDataLen = errors.New("read flv pre data len error")

func (fr *FlvReader) Read() (p *av.Packet, err error) {
	if !fr.inited {
		if _, err := io.ReadFull(fr.r, fr.tagHeaderBuf); err != nil {
			return nil, err
		} else if bytes.Equal(fr.tagHeaderBuf, FlvFirstHeader) {
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
	preDataLen := pio.I32BE(fr.tagHeaderBuf[:4])
	if uint32(preDataLen) != dataLen+headerLen {
		return nil, ErrPreDataLen
	}

	return p, fr.demuxer.DemuxH(p)
}
