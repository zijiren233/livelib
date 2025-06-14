package server

import (
	"context"
	"errors"
	"net"
	"sync/atomic"

	"github.com/zijiren233/livelib/protocol/rtmp"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Server struct {
	connBufferSize int32
	authFunc       AuthFunc
}

type AuthFunc func(ReqAppName, ReqChannelName string, IsPublisher bool) (*Channel, error)

type ServerConf func(*Server)

func WithConnBufferSize(bufferSize int32) ServerConf {
	return func(s *Server) {
		s.connBufferSize = bufferSize
	}
}

func NewRtmpServer(authFunc AuthFunc, c ...ServerConf) *Server {
	s := &Server{
		authFunc: authFunc,
	}
	for _, conf := range c {
		conf(s)
	}
	return s
}

func (s *Server) SetConnBufferSize(bufferSize int32) {
	atomic.StoreInt32(&s.connBufferSize, bufferSize)
}

var ErrAppAlreadyExists = errors.New("app already exists")

var ErrAppNotFount = errors.New("app not found")

func (s *Server) Serve(l net.Listener) error {
	for {
		netconn, err := l.Accept()
		if err != nil {
			continue
		}
		conn := core.NewConn(netconn, int(atomic.LoadInt32(&s.connBufferSize)))
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn *core.Conn) (err error) {
	if err := conn.HandshakeServer(); err != nil {
		conn.Close()
		return err
	}
	connServer := core.NewConnServer(conn)
	defer connServer.Close()

	if err = connServer.ReadInitMsg(); err != nil {
		return err
	}

	app, name := connServer.ConnInfo.App, connServer.PublishInfo.Name
	if s.authFunc == nil {
		panic("rtmp server auth func not implemented")
	}

	channel, err := s.authFunc(app, name, connServer.IsPublisher())
	if err != nil {
		return err
	}

	if connServer.IsPublisher() {
		reader := rtmp.NewReader(connServer)
		defer reader.Close()
		channel.PushStart(reader)
	} else {
		writer := rtmp.NewWriter(connServer)
		defer writer.Close()
		channel.AddPlayer(writer)
		_ = writer.SendPacket(context.Background())
	}

	return err
}
