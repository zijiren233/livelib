package hls

type TSItem struct {
	TsName   string
	SeqNum   int
	Duration int
	Data     []byte
}

func NewTSItem(tsName string, duration, seqNum int, b []byte) *TSItem {
	var item = new(TSItem)
	item.TsName = tsName
	item.SeqNum = seqNum
	item.Duration = duration
	item.Data = make([]byte, len(b))
	copy(item.Data, b)
	return item
}
