package internal

import (
	"net"
	"sync"
)

const (
	defaultQueueSize          = 64
	defaultResponseBufferSize = defaultBufferSize + 16
)

type TorrentRequest struct {
	Conn *net.TCPConn
	Msg  []byte
}

type TorrentResponse struct {
	Resp []byte
	Err  error
}

type TorrentRequestHandler struct {
	send chan TorrentRequest
	recv chan TorrentResponse
	done chan struct{}
}

func NewTorrentRequestHandler(buffer int) *TorrentRequestHandler {
	return &TorrentRequestHandler{
		send: make(chan TorrentRequest, buffer),
		recv: make(chan TorrentResponse, buffer),
		done: make(chan struct{}, 1),
	}
}

func (h *TorrentRequestHandler) exec() {
	defer close(h.send)
	defer close(h.recv)

	for {
		select {
		case req := <-h.send:
			go func() {
				resp := TorrentResponse{}

				_, resp.Err = req.Conn.Write(req.Msg)
				if resp.Err != nil {
					h.recv <- resp

					return
				}

				buf := make([]byte, defaultResponseBufferSize)
				n := 0
				n, resp.Err = req.Conn.Read(buf)

				resp.Resp = buf[:n]

				h.recv <- resp

				return
			}()
		case <-h.done:
			return
		}
	}
}

func (h *TorrentRequestHandler) close() {
	defer close(h.done)
}

func (h *TorrentRequestHandler) receiver() <-chan TorrentResponse {
	return h.recv
}

func (h *TorrentRequestHandler) sendPayload(conn *net.TCPConn, payload []byte) {
	h.send <- TorrentRequest{Conn: conn, Msg: payload}
}

var (
	torrentReqHandler     *TorrentRequestHandler
	onceTorrentReqHandler sync.Once
)

func GetTorrentRequestHandler() *TorrentRequestHandler {
	onceTorrentReqHandler.Do(func() {
		torrentReqHandler = NewTorrentRequestHandler(defaultQueueSize)
		go torrentReqHandler.exec()
	})

	return torrentReqHandler
}
