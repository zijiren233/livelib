package flv

import (
	"fmt"

	"github.com/zijiren233/livelib/av"
)

type FlvTagHeader struct {
	TagType           uint8
	DataSize          uint32
	Timestamp         uint32
	TimestampExtended uint8
	StreamID          uint32
	PreTagSzie        uint32
}

type mediaTag struct {
	/*
		SoundFormat: UB[4]
		0 = Linear PCM, platform endian
		1 = ADPCM
		2 = MP3
		3 = Linear PCM, little endian
		4 = Nellymoser 16-kHz mono
		5 = Nellymoser 8-kHz mono
		6 = Nellymoser
		7 = G.711 A-law logarithmic PCM
		8 = G.711 mu-law logarithmic PCM
		9 = reserved
		10 = AAC
		11 = Speex
		14 = MP3 8-Khz
		15 = Device-specific sound
		Formats 7, 8, 14, and 15 are reserved for internal use
		AAC is supported in Flash Player 9,0,115,0 and higher.
		Speex is supported in Flash Player 10 and higher.
	*/
	soundFormat uint8

	/*
		SoundRate: UB[2]
		Sampling rate
		0 = 5.5-kHz For AAC: always 3
		1 = 11-kHz
		2 = 22-kHz
		3 = 44-kHz
	*/
	soundRate uint8

	/*
		SoundSize: UB[1]
		0 = snd8Bit
		1 = snd16Bit
		Size of each sample.
		This parameter only pertains to uncompressed formats.
		Compressed formats always decode to 16 bits internally
	*/
	soundSize uint8

	/*
		SoundType: UB[1]
		0 = sndMono
		1 = sndStereo
		Mono or stereo sound For Nellymoser: always 0
		For AAC: always 1
	*/
	soundType uint8

	/*
		0: AAC sequence header
		1: AAC raw
	*/
	aacPacketType uint8

	/*
		1: keyframe (for AVC, a seekable frame)
		2: inter frame (for AVC, a non- seekable frame)
		3: disposable inter frame (H.263 only)
		4: generated keyframe (reserved for server use only)
		5: video info/command frame
	*/
	frameType uint8

	/*
		1: JPEG (currently unused)
		2: Sorenson H.263
		3: Screen video
		4: On2 VP6
		5: On2 VP6 with alpha channel
		6: Screen video version 2
		7: AVC
	*/
	codecID uint8

	/*
		0: AVC sequence header
		1: AVC NALU
		2: AVC end of sequence (lower level NALU sequence ender is not required or supported)
	*/
	avcPacketType uint8

	compositionTime int32
}

type FlvTagBody struct {
	mediat mediaTag
}

func (tag *FlvTagBody) SoundFormat() uint8 {
	return tag.mediat.soundFormat
}

func (tag *FlvTagBody) AACPacketType() uint8 {
	return tag.mediat.aacPacketType
}

func (tag *FlvTagBody) IsKeyFrame() bool {
	return tag.mediat.frameType == av.FRAME_KEY
}

func (tag *FlvTagBody) IsSeq() bool {
	return tag.mediat.frameType == av.FRAME_KEY &&
		tag.mediat.avcPacketType == av.AVC_SEQHDR
}

func (tag *FlvTagBody) CodecID() uint8 {
	return tag.mediat.codecID
}

func (tag *FlvTagBody) CompositionTime() int32 {
	return tag.mediat.compositionTime
}

// ParseMediaTagHeader, parse video, audio, tag header
func (tag *FlvTagBody) ParseMediaTagHeader(b []byte, isVideo bool) (n int, err error) {
	switch isVideo {
	case false:
		n, err = tag.parseAudioHeader(b)
	case true:
		n, err = tag.parseVideoHeader(b)
	}
	return
}

var ErrInvalidAudioData = fmt.Errorf("invalid audio data")
var ErrInvalidVideoData = fmt.Errorf("invalid video data")

func (tag *FlvTagBody) parseAudioHeader(b []byte) (n int, err error) {
	if len(b) < 1 {
		return 0, ErrInvalidAudioData
	}
	flags := b[0]
	tag.mediat.soundFormat = flags >> 4
	tag.mediat.soundRate = (flags >> 2) & 0x3
	tag.mediat.soundSize = (flags >> 1) & 0x1
	tag.mediat.soundType = flags & 0x1
	n++
	switch tag.mediat.soundFormat {
	case av.SOUND_AAC:
		if len(b) < 2 {
			return 1, ErrInvalidAudioData
		}
		tag.mediat.aacPacketType = b[1]
		n++
	}
	return
}

func (tag *FlvTagBody) parseVideoHeader(b []byte) (n int, err error) {
	if len(b) < 1 {
		return 0, ErrInvalidVideoData
	}
	flags := b[0]
	tag.mediat.frameType = flags >> 4
	tag.mediat.codecID = flags & 0x0f
	n++
	if tag.mediat.frameType == av.FRAME_INTER || tag.mediat.frameType == av.FRAME_KEY {
		switch tag.mediat.codecID {
		case av.CODEC_AVC:
			if len(b) < 5 {
				return 1, ErrInvalidVideoData
			}
			tag.mediat.avcPacketType = b[1]
			n++
			for _, v := range b[2:5] {
				tag.mediat.compositionTime = tag.mediat.compositionTime<<8 + int32(v)
				n++
			}
		}
	}
	return
}
