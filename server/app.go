package server

import (
	"errors"

	"github.com/zijiren233/ksync"
)

type app struct {
	appName      string
	channelsLock *ksync.Kmutex
	channels     map[string]*channel
}

func newApp(appName string) *app {
	return &app{
		appName:      appName,
		channelsLock: ksync.NewKmutex(),
		channels:     make(map[string]*channel),
	}
}

func (a *app) GetOrNewChannel(channelName string) *channel {
	a.channelsLock.Lock(channelName)
	defer a.channelsLock.Unlock(channelName)
	return a.getOrNewChannel(channelName)
}

func (a *app) getOrNewChannel(channelName string) *channel {
	if c, ok := a.channels[channelName]; ok {
		return c
	} else {
		c := newChannel(channelName)
		a.channels[channelName] = c
		return c
	}
}

var ErrChannelNotFound = errors.New("channel not found")

func (a *app) GetChannel(channelName string) (*channel, error) {
	a.channelsLock.Lock(channelName)
	defer a.channelsLock.Unlock(channelName)
	if c, ok := a.channels[channelName]; ok {
		return c, nil
	} else {
		return nil, ErrChannelNotFound
	}
}
