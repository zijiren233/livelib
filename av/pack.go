package av

import "errors"

var ErrClosed = errors.New("channel closed")

// Header can be converted to AudioHeaderInfo or VideoHeaderInfo
type Packet struct {
	IsAudio    bool
	IsVideo    bool
	IsMetadata bool
	TimeStamp  uint32 // dts
	StreamID   uint32
	Header     PacketHeader
	Data       []byte
}

func (p *Packet) Type() int {
	if p.IsVideo {
		return TAG_VIDEO
	} else if p.IsMetadata {
		return TAG_SCRIPTDATAAMF0
	} else {
		return TAG_AUDIO
	}
}

func (p *Packet) Clone() *Packet {
	var tp = *p
	tp.Data = make([]byte, len(p.Data))
	copy(tp.Data, p.Data)
	return &tp
}

const DropDefaultNum = 256

func DropPacket(pktQue chan *Packet) (n int) {
	return DropNPacket(pktQue, DropDefaultNum)
}

func DropNPacket(pktQue chan *Packet, dn int) (n int) {
	for {
		select {
		case _, ok := <-pktQue:
			if !ok {
				return n
			}
			n++
			// if p.IsAudio {
			// 	fmt.Println("IsAudio")
			// 	select {
			// 	case pktQue <- p:
			// 	default:
			// 		n++
			// 	}
			// 	if cap(pktQue)-2 >= len(pktQue) {
			// 		return n
			// 	}
			// 	continue
			// }

			// if p.IsVideo {
			// 	fmt.Println("IsVideo")
			// 	videoPkt, ok := p.Header.(VideoPacketHeader)
			// 	if ok && (videoPkt.IsSeq() || videoPkt.IsKeyFrame()) {
			// 		select {
			// 		case pktQue <- p:
			// 		default:
			// 			n++
			// 		}
			// 		if cap(pktQue)-2 >= len(pktQue) {
			// 			return n
			// 		}
			// 		continue
			// 	}
			// }
			if n == dn {
				return n
			}
		default:
			return n
		}
	}
}

type PacketHeader interface {
}

type AudioPacketHeader interface {
	PacketHeader
	SoundFormat() uint8
	AACPacketType() uint8
}

type VideoPacketHeader interface {
	PacketHeader
	IsKeyFrame() bool
	IsSeq() bool
	CodecID() uint8
	CompositionTime() int32
}
