package internal

import (
	"fmt"
	"iter"
	"log"
	"time"
)

const (
	defaultHandlerChanSize = 5
	defaultSendRetryTime   = 50 * time.Millisecond
	defaultReadTimeout     = 2000 * time.Millisecond
	defaultRequestBuffer   = 128
)

type TorrentManager struct {
	peers TorrentPeerPool
	recv  chan *TorrentPiece
	send  chan PieceMessage
	available chan *TorrentPeer
}

func newTorrentManager(info *TorrentInfo) (mng *TorrentManager, err error) {
	mng = &TorrentManager{}

	if mng.peers, err = newTorrentPeerPool(info); err != nil {
		err = fmt.Errorf("failed to connect with peers: %w")

		return
	}

	mng.recv = make(chan *TorrentPiece, len(mng.peers))
	mng.send = make(chan PieceMessage, 1)
	mng.available = make(chan *TorrentPeer, len(mng.peers))

	return
}

func (mng *TorrentManager) close() {
	mng.peers.close()
	close(mng.recv)
	close(mng.send)
}

func (p *TorrentManager) receive

type TorrentHandler struct {
	peer *TorrentPeer
	recv chan RequestMessage
	send chan PieceMessage
	done chan struct{}
	keys map[PieceKey]struct{}
}

func newTorrentHandler(
	peer *TorrentPeer,
	send chan PieceMessage,
) *TorrentHandler {
	return &TorrentHandler{
		peer: peer,
		recv: make(chan RequestMessage, defaultHandlerChanSize),
		send: send,
		done: make(chan struct{}, 1),
		keys: make(map[PieceKey]struct{}),
	}
}

func (h *TorrentHandler) sendRequest(msg RequestMessage) bool {
	if _, ok := h.keys[msg.key()]; ok {
		return false
	}

	select {
	case h.recv <- msg:
		h.keys[msg.key()] = struct{}{}

		return true
	default:
		return false
	}
}

func (h *TorrentHandler) addr() string {
	return h.peer.addr.String()
}

func (h *TorrentHandler) finalize() {
	h.done <- struct{}{}
}

func (h *TorrentHandler) queueRequest() (keys map[PieceKey]struct{}, buff []byte, n int) {
	writeBuff := [defaultRequestBuffer]byte{}
	keys = make(map[PieceKey]struct{}, defaultHandlerChanSize)
	offset := 0

loop:
	for range defaultHandlerChanSize {
		select {
		case msg := <-h.recv:
			keys[msg.key()] = struct{}{}
			offset += copy(writeBuff[offset:], msg.encode())
			n++
		case <-time.After(defaultSendRetryTime):
			break loop
		case <-h.done:
			return
		}
	}

	buff = writeBuff[:offset]

	return
}

func (h *TorrentHandler) exec() {
	for {
		keys, writeBuff, count := h.queueRequest()

		if count == 0 {
			time.After(defaultSendRetryTime)
		}

		// log.Printf("sending %d messages", count)

		h.peer.write(writeBuff)

		next, stop := iter.Pull(NewMessage(h.peer.reader))

		h.peer.setTimeout(defaultReadTimeout)

		for count > 0 {
			msg, ok := next()
			if !ok {
				break
			}

			if msg.Err != nil {
				// panic(msg.Err)
				log.Println(msg.Err)
				h.peer.reconnect()
				clear(h.keys)

				break
			}

			if msg.Type != Piece {
				log.Println(msg)
				panic("")
				// continue
			}

			// log.Println(hex.Dump(msg.content[:16]))

			payload := NewPiecePayload(msg.content)

			delete(keys, payload.key())

			h.send <- payload

			count--
		}

		h.peer.resetTimeout()

		for key := range keys {
			h.send <- PieceMessage{index: key.index, begin: key.begin, block: []byte{}}
		}

		stop()
	}
}

func (h *TorrentHandler) close() {
	h.finalize()
	h.peer.close()
	close(h.recv)
}

type TorrentHandlers struct {
	peers  []*TorrentHandler
	send   chan PieceMessage
	failed map[PieceKey]map[string]struct{}
}

func newTorrentHandlers(
	info *TorrentInfo,
) (handlers TorrentHandlers, err error) {
	peers, err := newTorrentPeerPool(info)
	if err != nil {
		return
	}

	handlers = TorrentHandlers{
		peers: make([]*TorrentHandler, len(peers)),
		send:  make(chan PieceMessage, 1),
	}

	// handlers = make(TorrentHandlers, len(peers))
	for i, peer := range peers {
		handlers.peers[i] = newTorrentHandler(peer, handlers.send)
	}

	return
}

func (h *TorrentHandlers) exec() {
	for _, handler := range h.peers {
		go handler.exec()
	}
}

func (h *TorrentHandlers) close() {
	for _, handler := range h.peers {
		handler.close()
	}

	close(h.send)
}

func (h *TorrentHandlers) sendRequest(msg RequestMessage) {
	for range 3 {
		for _, peer := range h.peers {
			if peer.sendRequest(msg) {
				return
			}
		}

		time.Sleep(defaultReadTimeout)
	}

	panic("failed to send request")
}
