package server

import (
	"errors"

	"github.com/zijiren233/ksync"
)

type App struct {
	appName      string
	channelsLock *ksync.Kmutex
	channels     map[string]*channel
	closed       bool
}

func NewApp(appName string) *App {
	return &App{
		appName:      appName,
		channelsLock: ksync.NewKmutex(),
		channels:     make(map[string]*channel),
	}
}

func (a *App) GetOrNewChannel(channelName string) *channel {
	a.channelsLock.Lock(channelName)
	defer a.channelsLock.Unlock(channelName)
	return a.getOrNewChannel(channelName)
}

func (a *App) getOrNewChannel(channelName string) *channel {
	if c, ok := a.channels[channelName]; ok {
		return c
	} else {
		c := newChannel(channelName)
		a.channels[channelName] = c
		return c
	}
}

var ErrChannelNotFound = errors.New("channel not found")

func (a *App) GetChannel(channelName string) (*channel, error) {
	a.channelsLock.Lock(channelName)
	defer a.channelsLock.Unlock(channelName)
	if c, ok := a.channels[channelName]; ok {
		return c, nil
	} else {
		return nil, ErrChannelNotFound
	}
}

func (a *App) GetChannels() map[string]*channel {
	return a.channels
}

func (a *App) DelChannel(channelName string) error {
	a.channelsLock.Lock(channelName)
	defer a.channelsLock.Unlock(channelName)
	return a.delChannel(channelName)
}

func (a *App) delChannel(channelName string) error {
	if c, ok := a.channels[channelName]; ok {
		c.Close()
		delete(a.channels, channelName)
		return nil
	} else {
		return ErrChannelNotFound
	}
}

func (a *App) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	for k := range a.channels {
		a.delChannel(k)
	}
	return nil
}
