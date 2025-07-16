package internal

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"iter"
	"log"
	"net"
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

type TorrentPeer struct {
	id        string
	owned     []byte
	requests  chan RequestMessage
	responses chan PieceMessage
	limiter   chan struct{}
	conn      *net.TCPConn
	addr      *net.TCPAddr
	reader    *bufio.Reader
	done      chan struct{}
}

func NewTorrentPeer(
	addr *net.TCPAddr,
	handshake []byte,
) (peer *TorrentPeer, err error) {
	peer = &TorrentPeer{
		requests:  make(chan RequestMessage, defaultPeerChanBuffer),
		responses: make(chan PieceMessage, defaultPeerChanBuffer),
		limiter:   make(chan struct{}, defaultPeerChanBuffer),
		addr:      addr,
		done:      make(chan struct{}),
	}

	if err := peer.dial(); err != nil {
		return peer, fmt.Errorf(
			"failed to establish connection with %q: %w",
			peer.addr,
			err,
		)
	}

	log.Println("conn established")

	if err = peer.handshake(handshake); err != nil {
		return peer, fmt.Errorf("error during handshake: %w", err)
	}

	log.Println("handshake performed")

	return peer, err
}

func (peer *TorrentPeer) write(msg []byte) error {
	totalWritten := 0

	for totalWritten < len(msg) {
		n, err := peer.conn.Write(msg[totalWritten:])
		if err != nil {
			return fmt.Errorf(
				"write failed after %d bytes: %w",
				totalWritten,
				err,
			)
		}

		if n == 0 {
			return fmt.Errorf("zero bytes written without error")
		}

		totalWritten += n
	}

	return nil
}

func (peer *TorrentPeer) dial() (err error) {
	peer.conn, err = net.DialTCP("tcp", nil, peer.addr)
	if err != nil {
		err = fmt.Errorf("error making connection: %s", err)

		return
	}

	peer.reader = bufio.NewReader(peer.conn)

	return
}

func (peer *TorrentPeer) interested() error {
	if err := peer.write(NewInterestedMsg().encode()); err != nil {
		return fmt.Errorf("error sending interested: %w", err)
	}

	found := false

	for msg := range NewMessage(peer.reader) {
		if msg.Err != nil {
			return fmt.Errorf("failed to read unchoke: %w", msg.Err)
		}

		if msg.Type == Unchoke {
			// log.Println("received unchoke")
			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf(
			"error sending interested: no unchoke message detected",
		)
	}

	return nil
}

func (peer *TorrentPeer) handshake(
	handshake []byte,
) error {
	if err := peer.write(handshake); err != nil {
		return err
	}

	next, stop := iter.Pull(NewMessage(peer.reader))
	defer stop()

	response, ok := next()

	if !ok || response.Err != nil {
		return fmt.Errorf(
			"failed to fetch initial handshake response: %w",
			response.Err,
		)
	}

	peer.id = hex.EncodeToString(response.content[response.Size-shaHashLength:])

	response, ok = next()

	if !ok {
		return nil
	}

	if response.Err != nil {
		return fmt.Errorf("error reading bitfield: %w", response.Err)
	}

	peer.owned = response.content

	err := peer.interested()

	return err
}

func (peer *TorrentPeer) close() {
	close(peer.done)
	peer.conn.Close()
	peer.conn = nil
	peer.reader = nil
}

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
