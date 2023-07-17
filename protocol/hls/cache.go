package hls

import (
	"bytes"
	"container/list"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

const (
	maxTSCacheNum = 3
)

type TSCacheItem struct {
	num int
	ll  *list.List
}

func NewTSCacheItem() *TSCacheItem {
	return &TSCacheItem{
		ll:  list.New(),
		num: maxTSCacheNum,
	}
}

// TODO: found data race, fix it
func (tcCacheItem *TSCacheItem) GenM3U8PlayList(tsBasePath string) *bytes.Buffer {
	var seq int
	var getSeq bool
	var maxDuration int
	m3u8body := bytes.NewBuffer(nil)
	for e := tcCacheItem.ll.Front(); e != nil; e = e.Next() {
		item := e.Value.(*TSItem)
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

func (tcCacheItem *TSCacheItem) PushItem(item *TSItem) {
	if tcCacheItem.ll.Len() == tcCacheItem.num {
		e := tcCacheItem.ll.Front()
		tcCacheItem.ll.Remove(e)
	}
	item.TsName = strings.TrimSuffix(item.TsName, ".ts")
	tcCacheItem.ll.PushBack(item)
}

func (tcCacheItem *TSCacheItem) GetItem(tsName string) (*TSItem, error) {
	tsName = strings.TrimSuffix(tsName, ".ts")
	for e := tcCacheItem.ll.Front(); e != nil; e = e.Next() {
		item := e.Value.(*TSItem)
		if item.TsName == tsName {
			return item, nil
		}
	}
	return nil, fs.ErrNotExist
}
