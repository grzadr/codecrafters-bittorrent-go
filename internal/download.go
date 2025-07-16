package internal

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"iter"
	"log"
	"os"
	"strconv"
	"sync"
)

const defaultFileMode = 0o777

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
		} else {
			log.Println("zero message")
			// continue
		}

		wg.Done()
	}
}

type Queue[T any] struct {
	items []T
}

func (q *Queue[T]) Enqueue(item T) {
	q.items = append(q.items, item)
}

func (q *Queue[T]) Dequeue() (item T, ok bool) {
	if len(q.items) == 0 {
		return
	}

	item = q.items[0]
	q.items = q.items[1:]
	ok = true

	return item, ok
}

type PieceKey struct {
	index int
	begin int
}

type pieceRegistry struct {
	// requests Queue[RequestMessage]
	sum Hash
	// length int
	chunks map[int][]byte
}

func newPieceRegistry() *pieceRegistry {
	reg := &pieceRegistry{}

	return reg
}

func downloadPiece(index, length int, peers TorrentPeers) (piece []byte) {
	// peerPools := peers.withPiece(index)
	// log.Printf("number of peer pools: %d", len(peerPools))
	handler := NewTorrentRequestHandler(defaultQueueSize)
	defer handler.Close()

	// responses := make([]PiecePayload, 0, chunks)

	var wg sync.WaitGroup

	piece = make([]byte, length)

	// iter :=

	i := 0

	picked := peers.pick(index)
	pickedLen := len(picked)

	for msg := range IterRequestMessage(index, length, defaultBufferSize) {
		wg.Add(1)

		shift := i % pickedLen

		handler.send <- TorrentRequest{
			Peers: append(picked[shift:], picked[:shift]...),
			Msg:   msg,
		}

		i++
	}

	go receivePiece(handler.recv, &wg, &piece)

	log.Printf("waiting for %d\n", i)

	wg.Wait()

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

	log.Printf("writing %d bytes to %q", len(piece), downloadPath)

	if err := os.WriteFile(downloadPath, piece, defaultFileMode); err != nil {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	if _, err := os.Stat(downloadPath); errors.Is(err, os.ErrNotExist) {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	log.Println("file saved")
}

func CmdDownload(downloadPath, torrentPath string) {
}
