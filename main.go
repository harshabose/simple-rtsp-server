package main

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/pion/rtp"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
)

var (
	ErrorWritingRTSPPackets = errors.New("error writing rtp packets to the server")
)

type StreamInfo struct {
	stream    *gortsplib.ServerStream
	publisher *gortsplib.ServerSession
}

type serverHandler struct {
	s       *gortsplib.Server
	mutex   sync.RWMutex
	streams map[string]*StreamInfo
}

func newServerHandler() *serverHandler {
	return &serverHandler{
		streams: make(map[string]*StreamInfo),
	}
}

func (sh *serverHandler) OnConnOpen(ctx *gortsplib.ServerHandlerOnConnOpenCtx) {
	log.Printf("conn opened")
}

func (sh *serverHandler) OnConnClose(ctx *gortsplib.ServerHandlerOnConnCloseCtx) {
	log.Printf("conn closed (%v)", ctx.Error)
}

func (sh *serverHandler) OnSessionOpen(ctx *gortsplib.ServerHandlerOnSessionOpenCtx) {
	log.Printf("session opened")
}

func (sh *serverHandler) OnSessionClose(ctx *gortsplib.ServerHandlerOnSessionCloseCtx) {
	log.Printf("session closed")

	sh.mutex.Lock()
	defer sh.mutex.Unlock()

	for path, info := range sh.streams {
		if ctx.Session == info.publisher {
			info.stream.Close()
			delete(sh.streams, path)
			log.Printf("publisher disconnected from path: %s", path)
			break
		}
	}
}

func (sh *serverHandler) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	log.Printf("describe request for path: %s", ctx.Path)

	sh.mutex.RLock()
	defer sh.mutex.RUnlock()

	info, ok := sh.streams[ctx.Path]
	if !ok {
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, nil
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, info.stream, nil
}

func (sh *serverHandler) OnAnnounce(ctx *gortsplib.ServerHandlerOnAnnounceCtx) (*base.Response, error) {
	log.Printf("announce request for path: %s", ctx.Path)

	sh.mutex.Lock()
	defer sh.mutex.Unlock()

	if info, ok := sh.streams[ctx.Path]; ok {
		info.stream.Close()
		info.publisher.Close()
	}

	sh.streams[ctx.Path] = &StreamInfo{
		stream:    gortsplib.NewServerStream(sh.s, ctx.Description),
		publisher: ctx.Session,
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func (sh *serverHandler) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	log.Printf("setup request for path: %s", ctx.Path)

	sh.mutex.RLock()
	defer sh.mutex.RUnlock()

	info, ok := sh.streams[ctx.Path]
	if !ok {
		return &base.Response{
			StatusCode: base.StatusNotFound,
		}, nil, nil
	}

	return &base.Response{
		StatusCode: base.StatusOK,
	}, info.stream, nil
}

func (sh *serverHandler) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	log.Printf("play request for path: %s", ctx.Path)

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func (sh *serverHandler) OnRecord(ctx *gortsplib.ServerHandlerOnRecordCtx) (*base.Response, error) {
	log.Printf("record request for path: %s", ctx.Path)

	sh.mutex.RLock()
	info := sh.streams[ctx.Path]
	sh.mutex.RUnlock()

	ctx.Session.OnPacketRTPAny(func(medi *description.Media, forma format.Format, pkt *rtp.Packet) {
		if err := info.stream.WritePacketRTP(medi, pkt); err != nil {
			fmt.Printf("Error: %s: %s\n", ErrorWritingRTSPPackets.Error(), err.Error())
		}
	})

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func main() {
	h := newServerHandler()
	h.s = &gortsplib.Server{
		Handler:           h,
		RTSPAddress:       ":8554",
		UDPRTPAddress:     ":8000",
		UDPRTCPAddress:    ":8001",
		MulticastIPRange:  "224.1.0.0/16",
		MulticastRTPPort:  8002,
		MulticastRTCPPort: 8003,
	}

	log.Printf("server is ready")
	panic(h.s.StartAndWait())
}
