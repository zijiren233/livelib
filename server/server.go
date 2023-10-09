package server

import (
	"errors"
	"net"
	"sync/atomic"

	"github.com/zijiren233/gencontainer/rwmap"
	"github.com/zijiren233/livelib/protocol/rtmp"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Server struct {
	apps                   rwmap.RWMap[string, *App]
	connBufferSize         int32
	parseChannelFunc       parseChannelFunc
	initHlsPlayer          bool
	autoCreateAppOrChannel bool
}

type parseChannelFunc func(ReqAppName, ReqChannelName string, IsPublisher bool) (TrueAppName string, TrueChannel string, err error)

func DefaultRtmpServer() *Server {
	return &Server{
		connBufferSize:         4096,
		initHlsPlayer:          false,
		autoCreateAppOrChannel: false,
	}
}

type ServerConf func(*Server)

func WithParseChannelFunc(f parseChannelFunc) ServerConf {
	return func(s *Server) {
		s.parseChannelFunc = f
	}
}

func WithConnBufferSize(bufferSize int32) ServerConf {
	return func(s *Server) {
		s.connBufferSize = bufferSize
	}
}

func WithInitHlsPlayer(init bool) ServerConf {
	return func(s *Server) {
		s.initHlsPlayer = init
	}
}

func WithAutoCreateAppOrChannel(auto bool) ServerConf {
	return func(s *Server) {
		s.autoCreateAppOrChannel = auto
	}
}

func NewRtmpServer(c ...ServerConf) *Server {
	s := DefaultRtmpServer()
	for _, conf := range c {
		conf(s)
	}
	return s
}

func (s *Server) SetParseChannelFunc(f parseChannelFunc) {
	s.parseChannelFunc = f
}

func (s *Server) SetConnBufferSize(bufferSize int32) {
	atomic.StoreInt32(&s.connBufferSize, bufferSize)
}

var (
	ErrAppAlreadyExists = errors.New("app already exists")
)

func (s *Server) NewApp(appName string) (*App, error) {
	a := NewApp(appName)
	_, loaded := s.apps.LoadOrStore(appName, a)
	if loaded {
		return nil, ErrAppAlreadyExists
	}
	return a, nil
}

func (s *Server) GetOrNewApp(appName string) *App {
	a, _ := s.apps.LoadOrStore(appName, NewApp(appName))
	return a
}

var ErrAppNotFount = errors.New("app not found")

func (s *Server) GetApp(appName string) (*App, error) {
	a, ok := s.apps.Load(appName)
	if !ok {
		return nil, ErrAppNotFount
	}
	return a, nil
}

func (s *Server) DelApp(appName string) error {
	a, loaded := s.apps.LoadAndDelete(appName)
	if !loaded {
		return ErrAppNotFount
	}
	return a.Close()
}

func (s *Server) GetChannelWithApp(appName, channelName string) (*Channel, error) {
	a, err := s.GetApp(appName)
	if err != nil {
		return nil, err
	}
	return a.GetChannel(channelName)
}

func (s *Server) GetOrNewChannelWithApp(appName, channelName string) (*Channel, error) {
	return s.GetOrNewApp(appName).GetOrNewChannel(channelName)
}

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
		return
	}
	var app, name = connServer.ConnInfo.App, connServer.PublishInfo.Name
	if s.parseChannelFunc != nil {
		app, name, err = s.parseChannelFunc(connServer.ConnInfo.App, connServer.PublishInfo.Name, connServer.IsPublisher())
		if err != nil {
			return err
		}
	}
	var channel *Channel
	if s.autoCreateAppOrChannel {
		channel, err = s.GetOrNewChannelWithApp(app, name)
		if err != nil {
			return err
		}
	} else {
		app, err := s.GetApp(app)
		if err != nil {
			return err
		}
		channel, err = app.GetChannel(name)
		if err != nil {
			return err
		}
	}
	if connServer.IsPublisher() {
		reader := rtmp.NewReader(connServer)
		defer reader.Close()
		if s.initHlsPlayer {
			if err := channel.InitHlsPlayer(); err != nil {
				return err
			}
		}
		channel.PushStart(reader)
	} else {
		writer := rtmp.NewWriter(connServer)
		defer writer.Close()
		channel.AddPlayer(writer)
		writer.SendPacket()
	}

	return
}
