package core

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	neturl "net/url"
	"strings"

	"github.com/zijiren233/livelib/av"
	"github.com/zijiren233/livelib/protocol/amf"
)

const (
	respResult     = "_result"
	onStatus       = "onStatus"
	publishStart   = "NetStream.Publish.Start"
	connectSuccess = "NetConnection.Connect.Success"
)

var ErrFail = errors.New("response err")

type ConnClient struct {
	transID    int
	url        string
	app        string
	title      string
	curcmdName string
	streamid   uint32
	isRTMPS    bool
	conn       *Conn
	encoder    *amf.Encoder
	decoder    *amf.Decoder
	bytesw     *bytes.Buffer
}

func NewConnClient() *ConnClient {
	return &ConnClient{
		transID: 1,
		bytesw:  bytes.NewBuffer(nil),
		encoder: new(amf.Encoder),
		decoder: new(amf.Decoder),
	}
}

func (connClient *ConnClient) DecodeBatch(r io.Reader, ver amf.Version) (ret []any, err error) {
	vs, err := connClient.decoder.DecodeBatch(r, ver)
	return vs, err
}

func (connClient *ConnClient) readRespMsg() error {
	for {
		rc, err := connClient.conn.Read()
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		switch rc.TypeID {
		case 20, 17:
			r := bytes.NewReader(rc.Data)
			vs, _ := connClient.decoder.DecodeBatch(r, amf.AMF0)

			for k, v := range vs {
				switch v := v.(type) {
				case string:
					switch connClient.curcmdName {
					case cmdConnect, cmdCreateStream:
						if v != respResult {
							return errors.New(v)
						}

					case cmdPublish:
						if v != onStatus {
							return ErrFail
						}
					}
				case float64:
					switch connClient.curcmdName {
					case cmdConnect, cmdCreateStream:
						id := int(v)

						switch k {
						case 1:
							if id != connClient.transID {
								return ErrFail
							}
						case 3:
							connClient.streamid = uint32(id)
						}
					case cmdPublish:
						if int(v) != 0 {
							return ErrFail
						}
					}
				case amf.Object:
					switch connClient.curcmdName {
					case cmdConnect:
						code, ok := v["code"]
						if ok && code.(string) != connectSuccess {
							return ErrFail
						}
					case cmdPublish:
						code, ok := v["code"]
						if ok && code.(string) != publishStart {
							return ErrFail
						}
					}
				}
			}

			return nil
		}
	}
}

func (connClient *ConnClient) writeMsg(args ...any) error {
	connClient.bytesw.Reset()
	for _, v := range args {
		if _, err := connClient.encoder.Encode(connClient.bytesw, v, amf.AMF0); err != nil {
			return err
		}
	}
	msg := connClient.bytesw.Bytes()
	c := &ChunkStream{
		Format:    0,
		CSID:      3,
		Timestamp: 0,
		TypeID:    20,
		StreamID:  connClient.streamid,
		Length:    uint32(len(msg)),
		Data:      msg,
	}
	err := connClient.conn.Write(c)
	if err != nil {
		return err
	}
	return connClient.conn.Flush()
}

func (connClient *ConnClient) writeConnectMsg() error {
	event := make(amf.Object)
	event["app"] = connClient.app
	event["type"] = "nonprivate"
	event["flashVer"] = "FMS.3.1"
	event["tcUrl"] = connClient.url
	connClient.curcmdName = cmdConnect

	if err := connClient.writeMsg(cmdConnect, connClient.transID, event); err != nil {
		return err
	}
	return connClient.readRespMsg()
}

func (connClient *ConnClient) writeCreateStreamMsg() error {
	connClient.transID++
	connClient.curcmdName = cmdCreateStream

	if err := connClient.writeMsg(cmdCreateStream, connClient.transID, nil); err != nil {
		return err
	}

	for {
		err := connClient.readRespMsg()
		if err == nil {
			return err
		}

		if errors.Is(err, ErrFail) {
			return err
		}
	}
}

func (connClient *ConnClient) writePublishMsg() error {
	connClient.transID++
	connClient.curcmdName = cmdPublish
	if err := connClient.writeMsg(cmdPublish, connClient.transID, nil, connClient.title, publishLive); err != nil {
		return err
	}
	return connClient.readRespMsg()
}

func (connClient *ConnClient) writePlayMsg() error {
	connClient.transID++
	connClient.curcmdName = cmdPlay
	if err := connClient.writeMsg(cmdPlay, 0, nil, connClient.title); err != nil {
		return err
	}
	return connClient.readRespMsg()
}

func (connClient *ConnClient) Start(rtmpURL, method string) error {
	u, err := neturl.Parse(rtmpURL)
	if err != nil {
		return err
	}
	connClient.url = rtmpURL
	path := strings.TrimLeft(u.Path, "/")
	ps := strings.SplitN(path, "/", 2)
	if len(ps) != 2 {
		return fmt.Errorf("u path err: %s", path)
	}
	connClient.app = ps[0]
	connClient.title = ps[1]
	if u.RawQuery != "" {
		connClient.title += "?" + u.RawQuery
	}
	if !strings.HasPrefix(u.Scheme, "rtmp") {
		return fmt.Errorf("rtmp url err: %s", rtmpURL)
	}
	connClient.isRTMPS = strings.EqualFold(u.Scheme, "rtmps")

	var conn net.Conn
	if connClient.isRTMPS {
		var config tls.Config
		enable_tls_verify := false
		if enable_tls_verify {
			roots, err := x509.SystemCertPool()
			if err != nil {
				return err
			}
			config.RootCAs = roots
		} else {
			config.InsecureSkipVerify = true
		}

		conn, err = tls.Dial("tcp", u.Host, &config)
		if err != nil {
			return err
		}
	} else {
		conn, err = net.Dial("tcp", u.Host)
		if err != nil {
			return err
		}
	}

	connClient.conn = NewConn(conn, 4*1024)

	if err := connClient.conn.HandshakeClient(); err != nil {
		return err
	}

	if err := connClient.writeConnectMsg(); err != nil {
		return err
	}
	if err := connClient.writeCreateStreamMsg(); err != nil {
		return err
	}

	switch method {
	case av.PUBLISH:
		if err := connClient.writePublishMsg(); err != nil {
			return err
		}
	case av.PLAY:
		if err := connClient.writePlayMsg(); err != nil {
			return err
		}
	}

	return nil
}

func (connClient *ConnClient) Write(c *ChunkStream) error {
	if c.TypeID == av.TAG_SCRIPTDATAAMF0 ||
		c.TypeID == av.TAG_SCRIPTDATAAMF3 {
		var err error
		if c.Data, err = amf.MetaDataReform(c.Data, amf.ADD); err != nil {
			return err
		}
		c.Length = uint32(len(c.Data))
	}
	return connClient.conn.Write(c)
}

func (connClient *ConnClient) Flush() error {
	return connClient.conn.Flush()
}

func (connClient *ConnClient) Read() (*ChunkStream, error) {
	return connClient.conn.Read()
}

func (connClient *ConnClient) GetInfo() (app, name, url string) {
	app = connClient.app
	name = connClient.title
	url = connClient.url
	return
}

func (connClient *ConnClient) GetStreamId() uint32 {
	return connClient.streamid
}

func (connClient *ConnClient) Close() error {
	return connClient.conn.Close()
}
