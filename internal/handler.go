package internal

import (
	"bufio"
	"fmt"
	"log"
	"time"
)

const (
	defaultQueueSize      = 64
	defaultByteBuffer     = 64 * 1024
	defaultTickInterval   = 100 * time.Millisecond
	defaultPeerChanBuffer = 5
)

type TorrentRequest struct {
	// Pool *TorrentPeers
	Peers []*PeerConnection
	Msg   RequestMessage
}

type TorrentResponse struct {
	Resp []byte
	Req  RequestMessage
	Err  error
	done bool
}

// type BufferedConn struct {
// 	conn   *net.TCPConn
// 	reader *bufio.Reader
// 	writer *bufio.Writer
// }

// func NewBufferedConn(conn *net.TCPConn) *BufferedConn {
// 	return &BufferedConn{
// 		conn:   conn,
// 		reader: bufio.NewReaderSize(conn, defaultTcpBuffer),
// 		writer: bufio.NewWriterSize(conn, defaultTcpBuffer),
// 	}
// }

func NewTorrentResponse(
	pool *PeerConnectionPool,
	msg RequestMessage,
) (resp TorrentResponse) {
	conn := pool.acquire()
	defer pool.release()

	if err := sendEncoded(conn, msg.encode()); err != nil {
		resp.Err = err

		return resp
	}

	reader := bufio.NewReaderSize(conn, defaultByteBuffer)

	// reader := bufio.NewReaderSize(conn, defaultByteBuffer)
	// lengthBuf := make([]byte, int32Size)
	// // resp.Resp = make([]byte, defaultTcpBuffer)
	// total := 0

	log.Println("reading in a loop")

	for msg := range NewMessage(reader) {
		if msg.Err != nil {
			resp.Err = fmt.Errorf("error reading message: %w", msg.Err)

			return resp
		}

		if msg.Type != Piece {
			log.Println(msg.Type)

			continue
		}

		resp.Resp = msg.content

		break
	}

	// for {
	// 	if _, err := io.ReadFull(reader, lengthBuf); err == io.EOF {
	// 		break
	// 	} else if err != nil {
	// 		resp.Err = fmt.Errorf("error reading length: %w", err)

	// 		return resp
	// 	}

	// 	length := bytesToInt(lengthBuf)

	// 	log.Printf("reading message of size %d", length)

	// 	if length == 0 {
	// 		log.Println("skip empty")

	// 		continue
	// 	}

	// 	msgByte, err := reader.ReadByte()
	// 	if err != nil {
	// 		resp.Err = fmt.Errorf("error reading msg type: %w", err)

	// 		return resp
	// 	}

	// 	log.Printf("read message byte: %08b", msgByte)

	// 	if msgType := MessageType(msgByte); msgType != Piece {
	// 		resp.Err = fmt.Errorf("expected piece, got %s", msgType)

	// 		return resp
	// 	}

	// 	// if length > 1 {
	// 	n := 0

	// 	resp.Resp = make([]byte, length-1)
	// 	if n, err = io.ReadFull(reader, resp.Resp); err != nil {
	// 		resp.Err = fmt.Errorf("error reading payload: %w", err)

	// 		return resp
	// 	}

	// 	total += n
	// 	// }

	// 	break
	// }

	// resp.Resp = resp.Resp[:total]

	return resp
	// buf := make([]byte, defaultResponseBufferSize)
	// offset := 0
	// total := 0
	// interested := false
	//
	//	for {
	//		if total == length+13 {
	//			break
	//		}
	//		n, err := conn.Read(buf)
	//		log.Printf("read new %d, total %d bytes for %+v", n, total, msg)
	//		if err == io.EOF {
	//			if interested {
	//				break
	//			} else {
	//				sendInterested(conn)
	//				interested = true
	//			}
	//		} else if err != nil {
	//			resp.Err = fmt.Errorf("error reading piece %+v: %w", msg, err)
	//			break
	//		}
	//		copy(resp.Resp[offset:], buf[:n])
	//		offset += n
	//		total += n
	//	}
	//
	// // n, err := conn.Read(buf)
	// // log.Printf("read %d bytes", n)
	// // if err != nil {
	// // 	resp.Err = fmt.Errorf("error reading piece %+v: %w", msg, err)
	// // 	return resp
	// // }
	// resp.Resp = resp.Resp[:total]
	// // log.Printf("finished reading piece %+v: %d bytes read", msg, total)
	// return resp
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
				// if req.Pool == nil {
				// 	log.Println("done")
				// 	h.recv <- TorrentResponse{done: true}
				// 	return
				// }
				for range 3 {
					for _, peer := range req.Peers {
						resp := NewTorrentResponse(peer.pool, req.Msg)

						if resp.Err != nil {
							panic(resp.Err)
						}

						if len(resp.Resp) == 0 {
							continue
						}

						h.recv <- resp

						return
					}

					time.Sleep(100 * time.Millisecond)
				}

				panic("failed to download piece")
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

func CmdHandshake(path, ip string) (id string) {
	torrent := ParseTorrentFile(path)

	handshake := NewHandshakeRequest(torrent.hash).encode()

	peer, err := NewTorrentPeer(ParsePeerIP(ip), handshake)
	if err != nil {
		panic(err)
	}

	defer peer.close()

	return fmt.Sprintf(
		"Peer ID: %s",
		peer.id,
	)
}
