package av

import (
	"io"
)

// Flv Tag Header
const (
	TAG_AUDIO          = 0x08
	TAG_VIDEO          = 0x09
	TAG_SCRIPTDATAAMF0 = 0x12
	TAG_SCRIPTDATAAMF3 = 0xf
)

const (
	MetadatAMF0  = 0x12
	MetadataAMF3 = 0xf
)

const (
	SOUND_MP3                   = 2
	SOUND_NELLYMOSER_16KHZ_MONO = 4
	SOUND_NELLYMOSER_8KHZ_MONO  = 5
	SOUND_NELLYMOSER            = 6
	SOUND_ALAW                  = 7
	SOUND_MULAW                 = 8
	SOUND_AAC                   = 10
	SOUND_SPEEX                 = 11

	SOUND_5_5Khz = 0
	SOUND_11Khz  = 1
	SOUND_22Khz  = 2
	SOUND_44Khz  = 3

	SOUND_8BIT  = 0
	SOUND_16BIT = 1

	SOUND_MONO   = 0
	SOUND_STEREO = 1

	AAC_SEQHDR = 0
	AAC_RAW    = 1
)

const (
	AVC_SEQHDR = 0
	AVC_NALU   = 1
	AVC_EOS    = 2
)

// Flv Video Tag Data Frame Type
const (
	FRAME_KEY   = 1
	FRAME_INTER = 2
	FRAME_DISPO = 3
)

// Flv Codec ID
const (
	CODEC_JPEG        = 1
	CODEC_SORENSON    = 2
	CODEC_SCREEN      = 3
	CODEC_ON2VP6      = 4
	CODEC_ON2VP6ALPHA = 5
	CODEC_SCREEN2     = 6
	CODEC_AVC         = 7
)

var (
	PUBLISH = "publish"
	PLAY    = "play"
)

type Demuxer interface {
	Demux(*Packet) (ret *Packet, err error)
}

type Muxer interface {
	Mux(*Packet, io.Writer) error
}

type SampleRater interface {
	SampleRate() (int, error)
}

type CodecParser interface {
	SampleRater
	Parse(*Packet, io.Writer) error
}

type Writer interface {
	Write(*Packet) error
}

type Reader interface {
	Read() (*Packet, error)
}

type ReadCloser interface {
	io.Closer
	Reader
}

type WriteCloser interface {
	io.Closer
	Writer
}
