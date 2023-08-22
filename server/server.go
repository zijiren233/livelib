package server

import (
	"context"
	"errors"
	"net"

	"github.com/zijiren233/ksync"
	"github.com/zijiren233/livelib/protocol/rtmp"
	"github.com/zijiren233/livelib/protocol/rtmp/core"
)

type Server struct {
	appsLock               *ksync.Kmutex
	apps                   map[string]*App
	connBufferSize         int
	parseChannelFunc       parseChannelFunc
	initHlsPlayer          bool
	autoCreateAppOrChannel bool
}

type parseChannelFunc func(ReqAppName, ReqChannelName string, IsPublisher bool) (TrueAppName string, TrueChannel string, err error)

func DefaultRtmpServer() *Server {
	return &Server{
		appsLock:               ksync.NewKmutex(),
		apps:                   make(map[string]*App),
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

func WithConnBufferSize(bufferSize int) ServerConf {
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

func (s *Server) GetOrNewApp(appName string) *App {
	s.appsLock.Lock(appName)
	defer s.appsLock.Unlock(appName)
	return s.getOrNewApp(appName)
}

func (s *Server) getOrNewApp(appName string) *App {
	if app, ok := s.apps[appName]; ok {
		return app
	} else {
		a := NewApp(appName)
		s.apps[appName] = a
		return a
	}
}

var ErrAppNotFount = errors.New("app not found")

func (s *Server) GetApp(appName string) (*App, error) {
	s.appsLock.Lock(appName)
	defer s.appsLock.Unlock(appName)
	return s.getApp(appName)
}

func (s *Server) getApp(appName string) (*App, error) {
	if a, ok := s.apps[appName]; ok {
		return a, nil
	} else {
		return nil, ErrAppNotFount
	}
}

func (s *Server) DelApp(appName string) error {
	s.appsLock.Lock(appName)
	defer s.appsLock.Unlock(appName)
	return s.delApp(appName)
}

func (s *Server) delApp(appName string) error {
	app, ok := s.apps[appName]
	if !ok {
		return ErrAppNotFount
	}
	return app.Close()
}

func (s *Server) GetChannelWithApp(appName, channelName string) (*Channel, error) {
	a, err := s.GetApp(appName)
	if err != nil {
		return nil, err
	}
	return a.GetChannel(channelName)
}

func (s *Server) GetOrNewChannelWithApp(appName, channelName string) *Channel {
	return s.GetOrNewApp(appName).GetOrNewChannel(channelName)
}

func (s *Server) Serve(l net.Listener) error {
	for {
		netconn, err := l.Accept()
		if err != nil {
			continue
		}
		conn := core.NewConn(netconn, s.connBufferSize)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn *core.Conn) (err error) {
	if err := conn.HandshakeServer(); err != nil {
		conn.Close()
		return err
	}
	connServer := core.NewConnServer(conn)

	if err = connServer.ReadInitMsg(); err != nil {
		conn.Close()
		return
	}
	var app, name = connServer.ConnInfo.App, connServer.PublishInfo.Name
	if s.parseChannelFunc != nil {
		app, name, err = s.parseChannelFunc(connServer.ConnInfo.App, connServer.PublishInfo.Name, connServer.IsPublisher())
		if err != nil {
			conn.Close()
			return err
		}
	}
	var channel *Channel
	if s.autoCreateAppOrChannel {
		channel = s.GetOrNewChannelWithApp(app, name)
	} else {
		app, err := s.GetApp(app)
		if err != nil {
			conn.Close()
			return err
		}
		channel, err = app.GetChannel(name)
		if err != nil {
			conn.Close()
			return err
		}
	}
	if connServer.IsPublisher() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reader := rtmp.NewReader(ctx, connServer)
		defer reader.Close()
		if s.initHlsPlayer {
			channel.InitHlsPlayer()
		}
		channel.PushStart(ctx, reader)
	} else {
		writer := rtmp.NewWriter(context.Background(), connServer)
		defer writer.Close()
		channel.AddPlayer(writer)
		writer.SendPacket(true)
	}

	return
}
