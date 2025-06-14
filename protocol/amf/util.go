package amf

import (
	"fmt"
	"io"
)

func WriteByte(w io.Writer, b byte) (err error) {
	bytes := make([]byte, 1)
	bytes[0] = b

	_, err = WriteBytes(w, bytes)

	return
}

func WriteBytes(w io.Writer, bytes []byte) (int, error) {
	return w.Write(bytes)
}

func ReadByte(r io.Reader) (byte, error) {
	bytes, err := ReadBytes(r, 1)
	if err != nil {
		return 0x00, err
	}

	return bytes[0], nil
}

func ReadBytes(r io.Reader, n int) ([]byte, error) {
	bytes := make([]byte, n)

	m, err := r.Read(bytes)
	if err != nil {
		return bytes, err
	}

	if m != n {
		return bytes, fmt.Errorf("decode read bytes failed: expected %d got %d", m, n)
	}

	return bytes, nil
}

func WriteMarker(w io.Writer, m byte) error {
	return WriteByte(w, m)
}

func ReadMarker(r io.Reader) (byte, error) {
	return ReadByte(r)
}

func AssertMarker(r io.Reader, checkMarker bool, m byte) error {
	if !checkMarker {
		return nil
	}

	marker, err := ReadMarker(r)
	if err != nil {
		return err
	}

	if marker != m {
		return fmt.Errorf("decode assert marker failed: expected %v got %v", m, marker)
	}

	return nil
}
