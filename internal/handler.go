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
	peers     TorrentPeerPool
	recv      chan *TorrentPiece
	done      chan *TorrentPiece
	available chan *TorrentPeer
}

func newTorrentManager(info *TorrentInfo) (mng *TorrentManager, err error) {
	mng = &TorrentManager{}

	if mng.peers, err = newTorrentPeerPool(info); err != nil {
		err = fmt.Errorf("failed to connect with peers: %w", err)

		return
	}

	mng.recv = make(chan *TorrentPiece, len(mng.peers))
	mng.done = make(chan *TorrentPiece, 1)
	mng.available = make(chan *TorrentPeer, len(mng.peers))

	for _, peer := range mng.peers {
		mng.available <- peer
	}

	return
}

func (mng *TorrentManager) close() {
	mng.peers.close()
	close(mng.recv)
	close(mng.done)
	close(mng.available)
}

func (mng *TorrentManager) download(piece *TorrentPiece, peer *TorrentPeer) {
	if piece.download(peer) {
		mng.done <- piece
	} else {
		piece.reset()
		mng.recv <- piece
	}

	mng.available <- peer
}

func (mng *TorrentManager) schedule(piece *TorrentPiece) {
	for {
		for peer := range mng.available {
			if piece.visited(peer.addr) {
				mng.available <- peer
			}

			go mng.download(piece, peer)

			return
		}

		time.Sleep(defaultSendRetryTime)
	}
}

func (mng *TorrentManager) queue() {
	for piece := range mng.recv {
		mng.schedule(piece)
	}
}

// func (mng *TorrentManager) schedule() {
// 	for piece := range mng.recv {
// 		for {

// 		}
// 	}
// }

type TorrentHandler struct {
	peer *TorrentPeer
	recv chan RequestMessage
	send chan PieceMessage
	done chan struct{}
	keys map[BlockKey]struct{}
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
		keys: make(map[BlockKey]struct{}),
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

func (h *TorrentHandler) finalize() {
	h.done <- struct{}{}
}

func (h *TorrentHandler) queueRequest() (keys map[BlockKey]struct{}, buff []byte, n int) {
	writeBuff := [defaultRequestBuffer]byte{}
	keys = make(map[BlockKey]struct{}, defaultHandlerChanSize)
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

		h.peer.write(writeBuff)

		next, stop := iter.Pull(NewMessage(h.peer.reader))

		h.peer.setTimeout(defaultReadTimeout)

		for count > 0 {
			msg, ok := next()
			if !ok {
				break
			}

			if msg.Err != nil {
				log.Println(msg.Err)
				h.peer.reconnect()
				clear(h.keys)

				break
			}

			if msg.Type != Piece {
				log.Println(msg)
				panic(fmt.Errorf("expected piece, found %q", msg.Type))
			}

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
