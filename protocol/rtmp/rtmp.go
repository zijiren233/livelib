package rtmp

import (
	"io"

	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

const (
	maxQueueNum           = 1024
	SAVE_STATICS_INTERVAL = 5000
)

type ChunkReader interface {
	Read() (*core.ChunkStream, error)
}

type ChunkReadCloser interface {
	io.Closer
	ChunkReader
}

type ChunkWriter interface {
	Write(*core.ChunkStream) error
}

type ChunkWriteCloser interface {
	io.Closer
	ChunkWriter
}

type ChunkReadWriteCloser interface {
	io.Closer
	ChunkReader
	ChunkWriter
}

type StaticsBW struct {
	StreamId               uint32
	VideoDatainBytes       uint64
	LastVideoDatainBytes   uint64
	VideoSpeedInBytesperMS uint64

	AudioDatainBytes       uint64
	LastAudioDatainBytes   uint64
	AudioSpeedInBytesperMS uint64

	LastTimestamp int64
}
