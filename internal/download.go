package internal

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"log"
	"net"
	"os"
	"slices"
	"sync"
	"time"
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

type BlockKey struct {
	index int
	begin int
}

type TorrentPiece struct {
	index        int
	checksum     Hash
	block        []byte
	visitedPeers []*net.TCPAddr
}

func NewTorrentPiece(
	index int,
	info *TorrentInfo,
) *TorrentPiece {
	size := info.pieceLength

	// last piece is either full pieceLength or remainder
	if index+1 == len(info.pieces) {
		size = chunkSize(info.length, size)
	}

	// piece =

	// keys = IterRequestMessage(index, size, defaultBlockSize)

	return &TorrentPiece{
		index:        index,
		checksum:     info.pieces[index],
		block:        make([]byte, size),
		visitedPeers: make([]*net.TCPAddr, 0),
	}
}

func (p *TorrentPiece) isCorrect() bool {
	hash := sha1.Sum(p.block)

	return bytes.Equal(hash[:], p.checksum[:])
}

func (p *TorrentPiece) insert(piece PieceMessage) {
	copy(p.block[piece.begin:], piece.block)
}

func (p *TorrentPiece) reset() {
	clear(p.block)
}

func (p *TorrentPiece) visited(addr *net.TCPAddr) bool {
	for _, visited := range p.visitedPeers {
		if visited.IP.Equal(addr.IP) && visited.Port == addr.Port {
			return true
		}
	}

	return false
}

func (p *TorrentPiece) addPeer(addr *net.TCPAddr) {
	p.visitedPeers = append(p.visitedPeers, addr)
}

func (p *TorrentPiece) iterRequestMessages() iter.Seq[RequestMessage] {
	return IterRequestMessage(p.index, len(p.block), defaultBlockSize)
}

func (p *TorrentPiece) iterChunks() iter.Seq2[map[BlockKey]struct{}, []byte] {
	return func(yield func(map[BlockKey]struct{}, []byte) bool) {
		writeBuff := [defaultRequestBuffer]byte{}
		keys := make(map[BlockKey]struct{}, defaultHandlerChanSize)

		blocks := slices.Collect(p.iterRequestMessages())

		for msg := range slices.Chunk(blocks, defaultHandlerChanSize) {
			clear(writeBuff[:])
			clear(keys)

			offset := 0

			for _, m := range msg {
				keys[m.key()] = struct{}{}
				offset += copy(writeBuff[offset:], m.encode())
			}

			if !yield(keys, writeBuff[:offset]) {
				return
			}
		}
	}
}

func (p *TorrentPiece) download(peer *TorrentPeer) bool {
	p.addPeer(peer.addr)

	for keys, msg := range p.iterChunks() {
		peer.write(msg)

		next, stop := iter.Pull(NewMessage(peer.reader))
		defer stop()

		peer.setTimeout(defaultReadTimeout)

		for len(keys) > 0 {
			response, ok := next()
			if !ok {
				break
			}

			if response.Err != nil || response.Type != Piece {
				// log.Println(response)
				peer.reconnect()

				return false
			}

			piece := NewPiecePayload(response.content)
			p.insert(piece)

			delete(keys, piece.key())

			// log.Println(piece.begin, piece.index, hex.Dump(piece.block[:16]))
		}

		peer.resetTimeout()
	}

	return p.isCorrect()
}

type TorrentFile struct {
	content     []byte
	pieces      []*TorrentPiece
	pieceLength int
	wg          *sync.WaitGroup
}

func newTorrentFile(info *TorrentInfo) (file *TorrentFile) {
	file = &TorrentFile{
		pieceLength: info.pieceLength,
		content:     make([]byte, info.length),
		pieces:      make([]*TorrentPiece, len(info.pieces)),
		wg:          &sync.WaitGroup{},
	}
	for index := range len(info.pieces) {
		file.wg.Add(1)
		file.pieces[index] = NewTorrentPiece(index, info)
	}

	return
}

func (file *TorrentFile) wait() {
	file.wg.Wait()
}

func (file *TorrentFile) schedule(mng *TorrentManager) {
	for _, piece := range file.pieces {
		mng.schedule(piece)
	}
}

func (file *TorrentFile) collect(mng *TorrentManager) {
	for piece := range mng.done {
		file.wg.Done()
		file.pieces[piece.index] = nil
		copy(file.content[piece.index*file.pieceLength:], piece.block)
	}
}

func (file *TorrentFile) write(path string) {
	if err := os.WriteFile(path, file.content, defaultFileMode); err != nil {
		panic(fmt.Errorf("error writing file to %q: %w", path, err))
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		panic(fmt.Errorf("error writing file to %q: %w", path, err))
	}
}

func CmdDownloadPiece(downloadPath, torrentPath string, index int) {
	log.Printf("downloading piece %d", index)

	info := ParseTorrentFile(torrentPath)

	mng, err := newTorrentManager(info)
	if err != nil {
		panic(err)
	}
	defer mng.close()

	piece := NewTorrentPiece(index, info)

	go mng.queue()
	mng.schedule(piece)

	select {
	case piece = <-mng.done:
	case <-time.After(defaultReadTimeout):
		panic(fmt.Errorf("piece %d download timeout", index))
	}

	log.Printf("writing %d bytes to %q", len(piece.block), downloadPath)

	if err := os.WriteFile(downloadPath, piece.block, defaultFileMode); err != nil {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	if _, err := os.Stat(downloadPath); errors.Is(err, os.ErrNotExist) {
		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	}

	log.Println("file saved")
}

func CmdDownload(downloadPath, torrentPath string) {
	info := ParseTorrentFile(torrentPath)

	mng, err := newTorrentManager(info)
	if err != nil {
		panic(err)
	}

	defer mng.close()

	go mng.queue()

	file := newTorrentFile(info)
	go file.schedule(mng)
	go file.collect(mng)
	file.wait()

	file.write(downloadPath)
}
