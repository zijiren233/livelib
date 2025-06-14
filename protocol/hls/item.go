package hls

type TSItem struct {
	TsName   string
	SeqNum   int64
	Duration int64
	Data     []byte
}

func NewTSItem(tsName string, duration, seqNum int64, b []byte) *TSItem {
	item := new(TSItem)
	item.TsName = tsName
	item.SeqNum = seqNum
	item.Duration = duration
	item.Data = make([]byte, len(b))
	copy(item.Data, b)
	return item
}
