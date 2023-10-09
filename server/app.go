package server

import (
	"errors"
	"sync/atomic"

	"github.com/zijiren233/gencontainer/rwmap"
)

type App struct {
	appName  string
	channels rwmap.RWMap[string, *Channel]
	closed   uint32
}

func NewApp(appName string) *App {
	return &App{
		appName: appName,
	}
}

var (
	ErrChannelAlreadyExists = errors.New("channel already exists")
)

func (a *App) NewChannel(channelName string) (*Channel, error) {
	if a.Closed() {
		return nil, ErrClosed
	}
	c := newChannel(channelName)
	_, loaded := a.channels.LoadOrStore(channelName, c)
	if loaded {
		return nil, ErrChannelAlreadyExists
	}
	return c, nil
}

func (a *App) GetOrNewChannel(channelName string) (*Channel, error) {
	if a.Closed() {
		return nil, ErrClosed
	}
	c, _ := a.channels.LoadOrStore(channelName, newChannel(channelName))
	return c, nil
}

var ErrChannelNotFound = errors.New("channel not found")

func (a *App) GetChannel(channelName string) (*Channel, error) {
	if a.Closed() {
		return nil, ErrClosed
	}
	c, ok := a.channels.Load(channelName)
	if !ok {
		return nil, ErrChannelNotFound
	}
	return c, nil

}

func (a *App) GetChannels() ([]*Channel, error) {
	if a.Closed() {
		return nil, ErrClosed
	}
	cs := make([]*Channel, 0)
	a.channels.Range(func(s string, c *Channel) bool {
		cs = append(cs, c)
		return true
	})
	return cs, nil
}

func (a *App) DelChannel(channelName string) error {
	if a.Closed() {
		return ErrClosed
	}
	c, ok := a.channels.LoadAndDelete(channelName)
	if !ok {
		return ErrChannelNotFound
	}
	return c.Close()
}

func (a *App) Close() error {
	if !atomic.CompareAndSwapUint32(&a.closed, 0, 1) {
		return nil
	}
	a.channels.Range(func(s string, c *Channel) bool {
		a.channels.Delete(s)
		c.Close()
		return true
	})
	return nil
}

func (a *App) Closed() bool {
	return atomic.LoadUint32(&a.closed) == 1
}
