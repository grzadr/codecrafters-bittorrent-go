package internal

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"iter"
	"log"
	"os"
	"strconv"
	"sync"
)

const defaultFileMode = 0o644

// type PieceSpec struct {
// 	index int
// 	begin int
// 	block int
// }

func IterRequestMessage(index, size, block int) iter.Seq[RequestMessage] {
	return func(yield func(RequestMessage) bool) {
		for num := range size / block {
			if !yield(
				RequestMessage{index: index, begin: num * block, block: block},
			) {
				return
			}
		}

		if left := size % block; left > 0 {
			if !yield(
				RequestMessage{index: index, begin: size - left, block: left},
			) {
				return
			}
		}
	}
}

func Cycle[S ~[]E, E any](items S) iter.Seq[E] {
	return func(yield func(E) bool) {
		for {
			for _, i := range items {
				if !yield(i) {
					return
				}
			}
		}
	}
}

func Zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq2[A, B] {
	return func(yield func(A, B) bool) {
		nextA, stopA := iter.Pull(a)
		defer stopA()

		nextB, stopB := iter.Pull(b)
		defer stopB()

		for {
			valueA, okA := nextA()
			if !okA {
				return
			}

			valueB, okB := nextB()
			if !okB {
				return
			}

			if !yield(valueA, valueB) {
				return
			}
		}
	}
}

// func IterRequestMsg(index, size, chunk int) iter.Seq[PieceSpec] {
// 	return func(yield func(PieceSpec) bool) {
// 		log.Printf("piece: %d %d %d", index, size, chunk)

// 		for spec := range IterRequestMessage(index, size, chunk) {
// 			log.Printf("spec: %+v\n", spec)

// 			if !yield(spec) {
// 				return
// 			}
// 		}
// 	}
// }

type PiecePayload struct {
	index int
	begin int
	block []byte
}

func NewPiecePayload(data []byte) (piece PiecePayload) {
	log.Println("NewPiecePayload\n", hex.Dump(data[:16]))
	// msg, _ := NewMessage(data)

	// if msg.msgType != Piece {
	// 	panic(fmt.Sprintf("expected %q, got %q", Piece, msg.msgType))
	// }

	log.Println("new message")

	piece.index = bytesToInt(data[:int32Size])
	piece.begin = bytesToInt(data[int32Size : int32Size*2])
	piece.block = data[int32Size*2:]

	return
}

func receivePiece(
	recv <-chan TorrentResponse,
	wg *sync.WaitGroup,
	piece *[]byte,
) {
	for resp := range recv {
		if resp.done {
			log.Println("receiver is done")

			return
		}

		if resp.Err != nil {
			panic(resp.Err)
		}

		if len(resp.Resp) != 0 {
			msg := NewPiecePayload(resp.Resp)

			log.Printf("copy %d %d\n", msg.index, msg.begin)

			copy((*piece)[msg.begin:], msg.block)
		}

		wg.Done()
	}
}

func downloadPiece(index, length int, peers TorrentPeers) (piece []byte) {
	peerPools := peers.withPiece(index)

	log.Printf("number of peer pools: %d", len(peerPools))

	handler := NewTorrentRequestHandler(defaultQueueSize)
	defer handler.Close()

	// responses := make([]PiecePayload, 0, chunks)

	var wg sync.WaitGroup

	piece = make([]byte, length)

	iter := IterRequestMessage(index, length, defaultBufferSize)

	i := 0

	for msg, pool := range Zip(iter, Cycle(peerPools)) {
		wg.Add(1)

		i++

		handler.send <- TorrentRequest{
			Pool: pool,
			Msg:  msg,
		}
	}

	go receivePiece(handler.recv, &wg, &piece)

	log.Printf("waiting for %d\n", i)

	wg.Wait()

	log.Println("wait complete")

	// handler.sendPayload(nil, nil)

	log.Println("returning piece")

	return piece
}

func CmdDownloadPiece(downloadPath, torrentPath, pieceIndex string) {
	torrent := ParseTorrentFile(torrentPath)

	index, _ := strconv.Atoi(pieceIndex)

	// NewHandshakeRequest(torrent.hash).make(addr)

	peers := NewTorrentPeersFromInfo(torrent)
	defer peers.close()

	piece := downloadPiece(index, torrent.pieceLength, peers)

	hash := sha1.Sum(piece)
	if !bytes.Equal(hash[:], torrent.pieces[index][:]) {
		panic("piece hash differ")
	}

	os.WriteFile(downloadPath, piece, defaultFileMode)
}

func CmdDownload(downloadPath, torrentPath string) {
}
