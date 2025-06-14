# livelib
golang live - rtmp - httpflv - hls

# How To Use

```go

go run . server --port 1935

```

You Can Use OBS to stream

- rtmp://127.0.0.1:1935/{app}
  - secret: {channel}

Then use any streaming address

- rtmp://127.0.0.1:1935/app
- http://127.0.0.1:1935/app/channel.flv
- http://127.0.0.1:1935/app/channel.m3u8

# Get command line help

```shell
go run . -h
```