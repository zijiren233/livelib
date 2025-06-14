package hls

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zijiren233/gencontainer/dllist"
)

const (
	maxTSCacheNum = 5
)

type TSCache struct {
	max  int
	l    *dllist.Dllist[*TSItem]
	lock sync.RWMutex
}

func NewTSCacheItem() *TSCache {
	return &TSCache{
		l:   dllist.New[*TSItem](),
		max: maxTSCacheNum,
	}
}

func (tc *TSCache) all() []*TSItem {
	tc.lock.RLock()
	defer tc.lock.RUnlock()
	items := make([]*TSItem, 0, tc.l.Len())
	for e := tc.l.Front(); e != nil; e = e.Next() {
		items = append(items, e.Value)
	}
	return items
}

func (tc *TSCache) GenM3U8File(tsPath func(tsName string) (tsPath string)) ([]byte, error) {
	var seq int64
	var maxDuration int64
	m3u8body := bytes.NewBuffer(nil)
	all := tc.all()
	if l := len(all); l > 3 {
		all = all[l-3:]
	}
	for _, item := range all {
		if item.Duration > maxDuration {
			maxDuration = item.Duration
		}
		if seq == 0 {
			seq = item.SeqNum
		}
		_, err := fmt.Fprintf(
			m3u8body,
			"#EXTINF:%.3f,\n%s\n",
			float64(item.Duration)/float64(1000),
			tsPath(item.TsName),
		)
		if err != nil {
			return nil, err
		}
	}
	w := bytes.NewBuffer(make([]byte, 0, m3u8body.Len()+256))
	fmt.Fprintf(
		w,
		"#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-ALLOW-CACHE:NO\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:%d\n",
		maxDuration/1000+1,
		seq,
	)
	_, err := m3u8body.WriteTo(w)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
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
	tsName = strings.TrimSuffix(tsName, filepath.Ext(tsName))
	tc.lock.RLock()
	defer tc.lock.RUnlock()
	for e := tc.l.Front(); e != nil; e = e.Next() {
		if e.Value.TsName == tsName {
			return e.Value, nil
		}
	}
	return nil, fs.ErrNotExist
}
