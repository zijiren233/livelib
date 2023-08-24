package hls

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/zijiren233/gencontainer/dllist"
)

const (
	maxTSCacheNum = 3
)

type TSCache struct {
	max  int
	l    *dllist.Dllist[*TSItem]
	lock *sync.RWMutex
}

func NewTSCacheItem() *TSCache {
	return &TSCache{
		l:    dllist.New[*TSItem](),
		max:  maxTSCacheNum,
		lock: new(sync.RWMutex),
	}
}

func (tc *TSCache) GenM3U8PlayList(tsBasePath string) *bytes.Buffer {
	var seq int
	var getSeq bool
	var maxDuration int
	m3u8body := bytes.NewBuffer(nil)
	tc.lock.RLock()
	defer tc.lock.RUnlock()
	for e := tc.l.Front(); e != nil; e = e.Next() {
		item := e.Value
		if item.Duration > maxDuration {
			maxDuration = item.Duration
		}
		if !getSeq {
			getSeq = true
			seq = item.SeqNum
		}
		fmt.Fprintf(m3u8body, "#EXTINF:%.3f,\n%s.ts\n", float64(item.Duration)/float64(1000), path.Join(tsBasePath, item.TsName))
	}
	w := bytes.NewBuffer(nil)
	fmt.Fprintf(w,
		"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-ALLOW-CACHE:NO\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n\n",
		maxDuration/1000+1, seq)
	m3u8body.WriteTo(w)
	return w
}

func (tc *TSCache) PushItem(item *TSItem) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	if tc.l.Len() == tc.max {
		e := tc.l.Front()
		tc.l.Remove(e)
	}
	item.TsName = strings.TrimSuffix(item.TsName, ".ts")
	tc.l.PushBack(item)
}

func (tc *TSCache) GetItem(tsName string) (*TSItem, error) {
	tsName = strings.TrimSuffix(tsName, ".ts")
	tc.lock.RLock()
	defer tc.lock.RUnlock()
	for e := tc.l.Front(); e != nil; e = e.Next() {
		item := e.Value
		if item.TsName == tsName {
			return item, nil
		}
	}
	return nil, fs.ErrNotExist
}
