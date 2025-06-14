package core

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/zijiren233/stream"
)

var timeout = 5 * time.Second

var (
	hsClientFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'P', 'l', 'a', 'y', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsServerFullKey = []byte{
		'G', 'e', 'n', 'u', 'i', 'n', 'e', ' ', 'A', 'd', 'o', 'b', 'e', ' ',
		'F', 'l', 'a', 's', 'h', ' ', 'M', 'e', 'd', 'i', 'a', ' ',
		'S', 'e', 'r', 'v', 'e', 'r', ' ',
		'0', '0', '1',
		0xF0, 0xEE, 0xC2, 0x4A, 0x80, 0x68, 0xBE, 0xE8, 0x2E, 0x00, 0xD0, 0xD1,
		0x02, 0x9E, 0x7E, 0x57, 0x6E, 0xEC, 0x5D, 0x2D, 0x29, 0x80, 0x6F, 0xAB,
		0x93, 0xB8, 0xE6, 0x36, 0xCF, 0xEB, 0x31, 0xAE,
	}
	hsClientPartialKey = hsClientFullKey[:30]
	hsServerPartialKey = hsServerFullKey[:36]
)

func hsMakeDigest(key, src []byte, gap int) (dst []byte) {
	h := hmac.New(sha256.New, key)
	if gap <= 0 {
		h.Write(src)
	} else {
		h.Write(src[:gap])
		h.Write(src[gap+32:])
	}
	return h.Sum(nil)
}

func hsCalcDigestPos(p []byte, base int) (pos int) {
	for i := range 4 {
		pos += int(p[base+i])
	}
	pos = (pos % 728) + base + 4
	return
}

func hsFindDigest(p, key []byte, base int) int {
	gap := hsCalcDigestPos(p, base)
	digest := hsMakeDigest(key, p, gap)
	if !bytes.Equal(p[gap:gap+32], digest) {
		return -1
	}
	return gap
}

func hsParse1(p, peerkey, key []byte) (ok bool, digest []byte) {
	var pos int
	if pos = hsFindDigest(p, peerkey, 772); pos == -1 {
		if pos = hsFindDigest(p, peerkey, 8); pos == -1 {
			return
		}
	}
	ok = true
	digest = hsMakeDigest(key, p[pos:pos+32], -1)
	return
}

func hsCreate01(p []byte, time, ver uint32, key []byte) {
	p[0] = 3
	p1 := p[1:]
	rand.Read(p1[8:])
	stream.BigEndian.WriteU32(p1[0:4], time)
	stream.BigEndian.WriteU32(p1[4:8], ver)
	gap := hsCalcDigestPos(p1, 8)
	digest := hsMakeDigest(key, p1, gap)
	copy(p1[gap:], digest)
}

func hsCreate2(p, key []byte) {
	rand.Read(p)
	gap := len(p) - 32
	digest := hsMakeDigest(key, p, gap)
	copy(p[gap:], digest)
}

func (conn *Conn) HandshakeClient() (err error) {
	var random [(1 + 1536*2) * 2]byte

	C0C1C2 := random[:1536*2+1]
	C0 := C0C1C2[:1]
	C0C1 := C0C1C2[:1536+1]

	S0S1S2 := random[1536*2+1:]

	C0[0] = 3
	// > C0C1
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.rw.Write(C0C1); err != nil {
		return err
	}
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if err = conn.rw.Flush(); err != nil {
		return err
	}

	// < S0S1S2
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = io.ReadFull(conn.rw, S0S1S2); err != nil {
		return err
	}

	S1 := S0S1S2[1 : 1536+1]
	var C2 []byte
	if ver := stream.BigEndian.ReadU32(S1[4:8]); ver != 0 {
		C2 = S1
	} else {
		C2 = S1
	}

	// > C2
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err = conn.rw.Write(C2); err != nil {
		return err
	}
	conn.Conn.SetDeadline(time.Time{})
	return err
}

func (conn *Conn) HandshakeServer() error {
	C := make([]byte, 1+1536*2)
	C0 := C[:1]
	C1 := C[1 : 1536+1]
	C0C1 := C[:1536+1]
	C2 := C[1536+1:]

	S := make([]byte, 1+1536*2)
	S0 := S[:1]
	S1 := S[1 : 1536+1]
	S0S1 := S[:1536+1]
	S2 := S[1536+1:]

	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err := io.ReadFull(conn.rw, C0C1); err != nil {
		return err
	}
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if C0[0] != 3 {
		return fmt.Errorf("rtmp: handshake version=%d invalid", C0[0])
	}

	S0[0] = 3

	servertime := stream.BigEndian.ReadU32(C1)
	serverVersion := uint32(0x0d0e0a0d)
	clientVersion := stream.BigEndian.ReadU32(C1[4:8])
	if clientVersion != 0 {
		if ok, digest := hsParse1(C1, hsClientPartialKey, hsServerFullKey); !ok {
			return errors.New("rtmp: handshake server: C1 invalid")
		} else {
			hsCreate01(S0S1, servertime, serverVersion, hsServerPartialKey)
			hsCreate2(S2, digest)
		}
	} else {
		copy(S1, C2)
		copy(S2, C1)
	}

	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.rw.Write(S); err != nil {
		return err
	}
	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if err := conn.rw.Flush(); err != nil {
		return err
	}

	conn.Conn.SetDeadline(time.Now().Add(timeout))
	if _, err := io.ReadFull(conn.rw, C2); err != nil {
		return err
	}
	conn.Conn.SetDeadline(time.Time{})
	return nil
}
