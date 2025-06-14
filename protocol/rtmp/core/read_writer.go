package core

import (
	"bufio"
	"io"
)

type ReadWriter struct {
	*bufio.ReadWriter
}

func NewReadWriter(rw io.ReadWriter, bufSize int) *ReadWriter {
	return &ReadWriter{
		ReadWriter: bufio.NewReadWriter(
			bufio.NewReaderSize(rw, bufSize),
			bufio.NewWriterSize(rw, bufSize),
		),
	}
}

func (rw *ReadWriter) Read(p []byte) (int, error) {
	n, err := io.ReadAtLeast(rw.ReadWriter, p, len(p))
	return n, err
}

func (rw *ReadWriter) ReadUintBE(n int) (uint32, error) {
	ret := uint32(0)
	for range n {
		b, err := rw.ReadByte()
		if err != nil {
			return 0, err
		}
		ret = ret<<8 + uint32(b)
	}
	return ret, nil
}

func (rw *ReadWriter) ReadUintLE(n int) (uint32, error) {
	ret := uint32(0)
	for i := range n {
		b, err := rw.ReadByte()
		if err != nil {
			return 0, err
		}
		ret += uint32(b) << uint32(i*8)
	}
	return ret, nil
}

func (rw *ReadWriter) Flush() error {
	return rw.ReadWriter.Flush()
}

func (rw *ReadWriter) Write(p []byte) (int, error) {
	return rw.ReadWriter.Write(p)
}

func (rw *ReadWriter) WriteUintBE(v uint32, n int) error {
	for i := range n {
		b := byte(v>>uint32((n-i-1)<<3)) & 0xff
		if err := rw.WriteByte(b); err != nil {
			return err
		}
	}
	return nil
}

func (rw *ReadWriter) WriteUintLE(v uint32, n int) error {
	for range n {
		b := byte(v) & 0xff
		if err := rw.WriteByte(b); err != nil {
			return err
		}
		v = v >> 8
	}
	return nil
}
