package core

import (
	"bytes"
	"fmt"
	"io"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
)

const (
	publishLive = "live"
)

var (
	ErrReq = fmt.Errorf("req error")
)

const (
	cmdConnect       = "connect"
	cmdFcpublish     = "FCPublish"
	cmdReleaseStream = "releaseStream"
	cmdCreateStream  = "createStream"
	cmdPublish       = "publish"
	cmdFCUnpublish   = "FCUnpublish"
	cmdDeleteStream  = "deleteStream"
	cmdPlay          = "play"
)

type ConnectInfo struct {
	App            string `amf:"app" json:"app"`
	Flashver       string `amf:"flashVer" json:"flashVer"`
	SwfUrl         string `amf:"swfUrl" json:"swfUrl"`
	TcUrl          string `amf:"tcUrl" json:"tcUrl"`
	Fpad           bool   `amf:"fpad" json:"fpad"`
	AudioCodecs    int    `amf:"audioCodecs" json:"audioCodecs"`
	VideoCodecs    int    `amf:"videoCodecs" json:"videoCodecs"`
	VideoFunction  int    `amf:"videoFunction" json:"videoFunction"`
	PageUrl        string `amf:"pageUrl" json:"pageUrl"`
	ObjectEncoding int    `amf:"objectEncoding" json:"objectEncoding"`
}

type ConnectResp struct {
	FMSVer       string `amf:"fmsVer"`
	Capabilities int    `amf:"capabilities"`
}

type ConnectEvent struct {
	Level          string `amf:"level"`
	Code           string `amf:"code"`
	Description    string `amf:"description"`
	ObjectEncoding int    `amf:"objectEncoding"`
}

type PublishInfo struct {
	Name string
	Type string
}

type ConnServer struct {
	done          bool
	streamID      int
	isPublisher   bool
	conn          *Conn
	transactionID int
	ConnInfo      ConnectInfo
	PublishInfo   PublishInfo
	decoder       *amf.Decoder
	encoder       *amf.Encoder
	bytesw        *bytes.Buffer
}

func NewConnServer(conn *Conn) *ConnServer {
	return &ConnServer{
		conn:     conn,
		streamID: 1,
		bytesw:   bytes.NewBuffer(nil),
		decoder:  &amf.Decoder{},
		encoder:  &amf.Encoder{},
	}
}

func (connServer *ConnServer) writeMsg(csid, streamID uint32, args ...interface{}) error {
	connServer.bytesw.Reset()
	for _, v := range args {
		if _, err := connServer.encoder.Encode(connServer.bytesw, v, amf.AMF0); err != nil {
			return err
		}
	}
	msg := connServer.bytesw.Bytes()
	c := ChunkStream{
		Format:    0,
		CSID:      csid,
		Timestamp: 0,
		TypeID:    20,
		StreamID:  streamID,
		Length:    uint32(len(msg)),
		Data:      msg,
	}
	connServer.conn.Write(&c)
	return connServer.conn.Flush()
}

func (connServer *ConnServer) connect(vs []interface{}) error {
	for _, v := range vs {
		switch v := v.(type) {
		case string:
		case float64:
			id := int(v)
			if id != 1 {
				return ErrReq
			}
			connServer.transactionID = id
		case amf.Object:
			if app, ok := v["app"]; ok {
				connServer.ConnInfo.App = app.(string)
			}
			if flashVer, ok := v["flashVer"]; ok {
				connServer.ConnInfo.Flashver = flashVer.(string)
			}
			if tcurl, ok := v["tcUrl"]; ok {
				connServer.ConnInfo.TcUrl = tcurl.(string)
			}
			if encoding, ok := v["objectEncoding"]; ok {
				connServer.ConnInfo.ObjectEncoding = int(encoding.(float64))
			}
		}
	}
	return nil
}

func (connServer *ConnServer) connectResp(CSID, StreamID uint32) error {
	c := connServer.conn.NewWindowAckSize(2500000)
	connServer.conn.Write(c)
	c = connServer.conn.NewSetPeerBandwidth(2500000)
	connServer.conn.Write(c)
	c = connServer.conn.NewSetChunkSize(uint32(1024))
	connServer.conn.Write(c)

	resp := make(amf.Object)
	resp["fmsVer"] = "FMS/3,0,1,123"
	resp["capabilities"] = 31

	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetConnection.Connect.Success"
	event["description"] = "Connection succeeded."
	event["objectEncoding"] = connServer.ConnInfo.ObjectEncoding
	return connServer.writeMsg(CSID, StreamID, "_result", connServer.transactionID, resp, event)
}

func (connServer *ConnServer) createStream(vs []interface{}) error {
	for _, v := range vs {
		switch v := v.(type) {
		case string:
		case float64:
			connServer.transactionID = int(v)
		case amf.Object:
		}
	}
	return nil
}

func (connServer *ConnServer) createStreamResp(CSID, StreamID uint32) error {
	return connServer.writeMsg(CSID, StreamID, "_result", connServer.transactionID, nil, connServer.streamID)
}

func (connServer *ConnServer) publishOrPlay(vs []interface{}) error {
	for k, v := range vs {
		switch v := v.(type) {
		case string:
			if k == 2 {
				connServer.PublishInfo.Name = v
			} else if k == 3 {
				connServer.PublishInfo.Type = v
			}
		case float64:
			connServer.transactionID = int(v)
		case amf.Object:
		}
	}

	return nil
}

func (connServer *ConnServer) publishResp(CSID, StreamID uint32) error {
	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Publish.Start"
	event["description"] = "Start publishing."
	return connServer.writeMsg(CSID, StreamID, "onStatus", 0, nil, event)
}

func (connServer *ConnServer) playResp(CSID, StreamID uint32) error {
	connServer.conn.SetRecorded()
	connServer.conn.SetBegin()

	event := make(amf.Object)
	event["level"] = "status"
	event["code"] = "NetStream.Play.Reset"
	event["description"] = "Playing and resetting stream."
	if err := connServer.writeMsg(CSID, StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = "status"
	event["code"] = "NetStream.Play.Start"
	event["description"] = "Started playing stream."
	if err := connServer.writeMsg(CSID, StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = "status"
	event["code"] = "NetStream.Data.Start"
	event["description"] = "Started playing stream."
	if err := connServer.writeMsg(CSID, StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}

	event["level"] = "status"
	event["code"] = "NetStream.Play.PublishNotify"
	event["description"] = "Started playing notify."
	if err := connServer.writeMsg(CSID, StreamID, "onStatus", 0, nil, event); err != nil {
		return err
	}
	return connServer.conn.Flush()
}

func (connServer *ConnServer) handleCmdMsg(c *ChunkStream) error {
	if c.TypeID == 17 {
		c.Data = c.Data[1:]
	}
	r := bytes.NewReader(c.Data)
	vi, err := connServer.decoder.DecodeBatch(r, amf.Version(amf.AMF0))
	if err != nil && err != io.EOF {
		return err
	}

	switch v := vi[0].(type) {
	case string:
		switch v {
		case cmdConnect:
			if err = connServer.connect(vi[1:]); err != nil {
				return err
			}
			if err = connServer.connectResp(c.CSID, c.StreamID); err != nil {
				return err
			}
		case cmdCreateStream:
			if err = connServer.createStream(vi[1:]); err != nil {
				return err
			}
			if err = connServer.createStreamResp(c.CSID, c.StreamID); err != nil {
				return err
			}
		case cmdPublish:
			if err = connServer.publishOrPlay(vi[1:]); err != nil {
				return err
			}
			if err = connServer.publishResp(c.CSID, c.StreamID); err != nil {
				return err
			}
			connServer.done = true
			connServer.isPublisher = true
		case cmdPlay:
			if err = connServer.publishOrPlay(vi[1:]); err != nil {
				return err
			}
			if err = connServer.playResp(c.CSID, c.StreamID); err != nil {
				return err
			}
			connServer.done = true
			connServer.isPublisher = false
		case cmdFcpublish:
			// connServer.fcPublish(vi)
		case cmdReleaseStream:
			// connServer.releaseStream(vi)
		case cmdFCUnpublish:
		case cmdDeleteStream:
		default:
		}
	}

	return nil
}

func (connServer *ConnServer) ReadInitMsg() error {
	for {
		if c, err := connServer.Read(); err != nil {
			return err
		} else {
			switch c.TypeID {
			case 20, 17:
				if err := connServer.handleCmdMsg(c); err != nil {
					return err
				}
			}
		}
		if connServer.done {
			break
		}
	}
	return nil
}

func (connServer *ConnServer) IsPublisher() bool {
	return connServer.isPublisher
}

func (connServer *ConnServer) Write(c *ChunkStream) error {
	if c.TypeID == av.TAG_SCRIPTDATAAMF0 ||
		c.TypeID == av.TAG_SCRIPTDATAAMF3 {
		var err error
		if c.Data, err = amf.MetaDataReform(c.Data, amf.DEL); err != nil {
			return err
		}
		c.Length = uint32(len(c.Data))
	}
	return connServer.conn.Write(c)
}

func (connServer *ConnServer) Flush() error {
	return connServer.conn.Flush()
}

func (connServer *ConnServer) Read() (*ChunkStream, error) {
	return connServer.conn.Read()
}

func (connServer *ConnServer) GetInfo() (app string, name string) {
	app = connServer.ConnInfo.App
	name = connServer.PublishInfo.Name
	return
}

func (connServer *ConnServer) Close() error {
	return connServer.conn.Close()
}
