package hls

import "errors"

const (
	duration = 3000
)

var (
	ErrNoPublisher         = errors.New("no publisher")
	ErrInvalidReq          = errors.New("invalid req url path")
	ErrNoSupportVideoCodec = errors.New("no support video codec")
	ErrNoSupportAudioCodec = errors.New("no support audio codec")
)

const (
	CrossdomainXML = `<?xml version="1.0" ?>
<cross-domain-policy>
	<allow-access-from domain="*" />
	<allow-http-request-headers-from domain="*" headers="*"/>
</cross-domain-policy>`
	M3U8ContentType = "application/x-mpegURL"
	TSContentType   = "video/mp2ts"
)
