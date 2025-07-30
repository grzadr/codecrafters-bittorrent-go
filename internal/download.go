package internal

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"iter"
	"log"
	"os"
)

const (
	defaultFileMode  = 0o644
	defaultBlockSize = 16 * 1024
)

func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

func chunkSize(size, chunk int) int {
	return (size-1)%chunk + 1
}

func IterRequestMessage(index, size, blockSize int) iter.Seq[RequestMessage] {
	return func(yield func(RequestMessage) bool) {
		numBlocks := ceilDiv(size, blockSize)

		for num := range numBlocks - 1 {
			if !yield(
				RequestMessage{
					index: index,
					begin: num * blockSize,
					block: blockSize,
				},
			) {
				return
			}
		}

		yield(
			RequestMessage{
				index: index,
				begin: (numBlocks - 1) * blockSize,
				block: chunkSize(size, blockSize),
			},
		)
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

type PieceBlock struct {
	checksum  Hash
	numBlocks int
	block     []byte
}

func NewPieceBlock(checksum Hash, size, blockSize int) (block *PieceBlock) {
	block = &PieceBlock{
		checksum: checksum,
		block:    make([]byte, size),
	}

	block.numBlocks = size / blockSize

	if size%blockSize != 0 {
		block.numBlocks++
	}

	return
}

type PieceKey struct {
	index int
	begin int
}

type TorrentPiece struct {
	checksum Hash
	block    []byte
}

func NewTorrentPiece(
	index int,
	info *TorrentInfo,
) (piece *TorrentPiece, keys iter.Seq[RequestMessage]) {
	size := info.pieceLength

	// last piece is either full pieceLength or remainder
	if index+1 == len(info.pieces) {
		size = chunkSize(info.length, size)
	}

	piece = &TorrentPiece{
		checksum: info.pieces[index],
		block:    make([]byte, size),
	}

	keys = IterRequestMessage(index, size, defaultBlockSize)

	return
}

func (p TorrentPiece) verify() {
	hash := sha1.Sum(p.block)
	//
	if !bytes.Equal(hash[:], p.checksum[:]) {
		panic("piece hash differ")
	}
}

type TorrentIndex struct {
	checksum Hash
	requests map[PieceKey]RequestMessage
	keys     map[PieceKey]*TorrentPiece
	pieces   []*TorrentPiece
	send     chan PieceMessage
	done     chan CompletedKey
	finish   chan struct{}
}

type CompletedKey struct {
	PieceKey
	ok   bool
	addr string
}

func (k CompletedKey) key() PieceKey {
	return k.PieceKey
}

func newTorrentIndexEmpty(
	info *TorrentInfo,
	send chan PieceMessage,
) (index *TorrentIndex) {
	index = &TorrentIndex{
		requests: make(
			map[PieceKey]RequestMessage,
			ceilDiv(info.length, defaultBlockSize),
		),
		pieces: make([]*TorrentPiece, 0, len(info.pieces)),
		send:   send,
		keys:   make(map[PieceKey]*TorrentPiece),
		done:   make(chan CompletedKey),
		finish: make(chan struct{}),
	}

	return
}

func newTorrentIndex(info *TorrentInfo,
	send chan PieceMessage,
) (index *TorrentIndex) {
	index = newTorrentIndexEmpty(info, send)

	for num := range len(info.pieces) {
		piece, iter := NewTorrentPiece(num, info)

		index.pieces = append(index.pieces, piece)

		for msg := range iter {
			index.requests[msg.key()] = msg
			index.keys[msg.key()] = piece
		}

		log.Printf("added %d piece of size %d", num, len(piece.block))
	}

	return
}

func newTorrentIndexSingle(
	num int,
	info *TorrentInfo,
	send chan PieceMessage,
) (index *TorrentIndex) {
	index = newTorrentIndexEmpty(info, send)
	piece, iter := NewTorrentPiece(num, info)

	log.Printf("added %d piece of size %d", num, len(piece.block))

	index.pieces = append(index.pieces, piece)

	for msg := range iter {
		index.requests[msg.key()] = msg
		index.keys[msg.key()] = piece
	}

	return
}

func (i *TorrentIndex) request(handlers TorrentHandlers) {
	defer i.close()

	for len(i.requests) > 0 {
		for _, msg := range i.requests {
			handlers.sendRequest(msg)
		}

		count := 0

		for range len(i.requests) {
			key := <-i.done
			log.Println("received", key)

			if key.ok {
				log.Println("deleting", key)
				delete(i.requests, key.key())
			}

			count++
		}

		log.Printf("completed %d requests", count)
	}

	i.finish <- struct{}{}
}

func (i *TorrentIndex) collect() {
	counter := 0

	for msg := range i.send {
		completed := CompletedKey{
			PieceKey: msg.key(),
			ok:       len(msg.block) != 0,
		}
		i.done <- completed

		if !completed.ok {
			continue
		}

		piece := i.keys[msg.key()]
		copy(piece.block[msg.begin:], msg.block)

		counter++
		log.Printf("received %d", counter)
	}
}

func (i *TorrentIndex) close() {
	close(i.done)
	close(i.finish)
}

func (i *TorrentIndex) wait() {
	log.Println("waiting")
	<-i.finish
	log.Println("finished")
}

func downloadPiece(
	num int,
	info *TorrentInfo,
	handlers TorrentHandlers,
) []byte {
	index := newTorrentIndexSingle(num, info, handlers.send)

	log.Printf("added %d requests\n", len(index.requests))

	go index.collect()
	go index.request(handlers)

	log.Println("waiting")

	index.wait()

	log.Println("wait complete")

	log.Println(index.pieces)

	piece := index.pieces[0]
	piece.verify()

	return piece.block
}

func CmdDownloadPiece(downloadPath, torrentPath string, index int) {
	log.Printf("downloading piece %d", index)

	info := ParseTorrentFile(torrentPath)

	handlers, err := newTorrentHandlers(info)
	if err != nil {
		panic(err)
	}

	defer handlers.close()
	handlers.exec()

	piece := downloadPiece(index, info, handlers)
	log.Printf("writing %d bytes to %q", len(piece), downloadPath)

	if err := os.WriteFile(downloadPath, piece, defaultFileMode); err != nil {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	if _, err := os.Stat(downloadPath); errors.Is(err, os.ErrNotExist) {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	log.Println("file saved")
}

func downloadFile(
	info *TorrentInfo,
	handlers TorrentHandlers,
) []byte {
	log.Printf("downloading %d bytes\n", info.length)
	index := newTorrentIndex(info, handlers.send)

	log.Printf("added %d requests\n", len(index.requests))

	go index.collect()
	go index.request(handlers)

	index.wait()

	log.Println(index.pieces)

	buffer := make([]byte, info.length)
	offset := 0

	for _, piece := range index.pieces {
		piece.verify()
		offset += copy(buffer[offset:], piece.block)
	}

	return buffer
}

func CmdDownload(downloadPath, torrentPath string) {
	info := ParseTorrentFile(torrentPath)

	handlers, err := newTorrentHandlers(info)
	if err != nil {
		panic(err)
	}

	defer handlers.close()
	handlers.exec()

	piece := downloadFile(info, handlers)

	if err := os.WriteFile(downloadPath, piece, defaultFileMode); err != nil {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	if _, err := os.Stat(downloadPath); errors.Is(err, os.ErrNotExist) {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	log.Println("file saved")
}
