package internal

import (
	"fmt"
	"io"
	"log"
	"time"
)

const (
	defaultQueueSize          = 64
	defaultResponseBufferSize = defaultBufferSize + 1024
	defaultTickInterval       = 100 * time.Millisecond
)

type TorrentRequest struct {
	Pool   *PeerConnectionPool
	Msg    []byte
	Length int
}

type TorrentResponse struct {
	Resp []byte
	Err  error
	done bool
}

func NewTorrentResponse(
	pool *PeerConnectionPool,
	msg []byte,
	length int,
) (resp TorrentResponse) {
	conn := pool.acquire()
	defer pool.release(conn)

	// if err := sendInterested(conn); err != nil {
	// 	resp.Err = fmt.Errorf("error sending interested: %w", err)

	// 	return resp
	// }

	if n, err := conn.Write(msg); err != nil {
		resp.Err = fmt.Errorf("error writing request: %w", err)

		return resp
	} else if n != len(msg) {
		resp.Err = fmt.Errorf("error writing %d bytes: wrote %d", len(msg), n)

		return resp
	}

	buf := make([]byte, defaultResponseBufferSize)

	offset := 0
	resp.Resp = make([]byte, defaultResponseBufferSize)

	total := 0

	for {
		if total == length+13 {
			break
		}

		n, err := conn.Read(buf)
		log.Printf("read new %d, total %d bytes for %+v", n, total, msg)

		if err == io.EOF {
			break
		} else if err != nil {
			resp.Err = fmt.Errorf("error reading piece %+v: %w", msg, err)

			break
		}

		copy(resp.Resp[offset:], buf[:n])

		offset += n
		total += n
	}

	// n, err := conn.Read(buf)
	// log.Printf("read %d bytes", n)

	// if err != nil {
	// 	resp.Err = fmt.Errorf("error reading piece %+v: %w", msg, err)

	// 	return resp
	// }

	resp.Resp = resp.Resp[:total]

	log.Printf("finished reading piece %+v: %d bytes read", msg, total)

	return resp
}

type TorrentRequestHandler struct {
	send chan TorrentRequest
	recv chan TorrentResponse
	done chan struct{}
}

func NewTorrentRequestHandler(buffer int) (handler *TorrentRequestHandler) {
	handler = &TorrentRequestHandler{
		send: make(chan TorrentRequest, buffer),
		recv: make(chan TorrentResponse, buffer),
		done: make(chan struct{}, 1),
	}

	go handler.exec()

	return handler
}

func (h *TorrentRequestHandler) Close() {
	defer close(h.done)
}

func (h *TorrentRequestHandler) exec() {
	ticker := time.NewTicker(defaultTickInterval)
	defer ticker.Stop()
	defer close(h.send)
	defer close(h.recv)

	for {
		select {
		case req := <-h.send:
			log.Printf("received request: %+v\n", req)

			go func() {
				if req.Pool == nil {
					log.Println("done")
					h.recv <- TorrentResponse{done: true}

					return
				}

				h.recv <- NewTorrentResponse(req.Pool, req.Msg, req.Length)
			}()
		case <-h.done:
			return
		default:
			time.Sleep(defaultTickInterval)
		}
	}
}

// func (h *TorrentRequestHandler) sendPayload(
// 	pool *PeerConnectionPool,
// 	payload []byte,
// ) {
// 	h.send <- TorrentRequest{Pool: pool, Msg: payload, expected}
// }
