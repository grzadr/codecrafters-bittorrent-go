package internal

import (
	"iter"
)

const (
	defaultFileMode  = 0o644
	defaultBlockSize = 16 * 1024
)

// type PieceSpec struct {
// 	index int
// 	begin int
// 	block int
// }

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
				begin: numBlocks * blockSize,
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

// func receivePiece(
// 	recv <-chan TorrentResponse,
// 	wg *sync.WaitGroup,
// 	piece *[]byte,
// ) {
// 	for resp := range recv {
// 		if resp.done {
// 			log.Println("receiver is done")

// 			return
// 		}

// 		if resp.Err != nil {
// 			panic(resp.Err)
// 		}

// 		if len(resp.Resp) != 0 {
// 			msg := NewPiecePayload(resp.Resp)

// 			log.Printf("copy %d %d\n", msg.index, msg.begin)

// 			copy((*piece)[msg.begin:], msg.block)
// 		} else {
// 			log.Println("zero message")
// 			// continue
// 		}

// 		wg.Done()
// 	}
// }

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
	checksum Hash
	// size      int
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

// func IterPieceKeys(index, size, blockSize int) iter.Seq2[PieceKey, int] {
// 	return func(yield func(PieceKey, int) bool) {
// 		numBlocks := ceilDiv(size, blockSize)

// 		for num := range numBlocks - 1 {
// 			if !yield(
// 				PieceKey{index: index, begin: num * blockSize}, block,
// 			) {
// 				return
// 			}
// 		}

// 		yield(
// 			PieceKey{
// 				index: index,
// 				begin: num * blockSize,
// 			},
// 			chunkSize(size, block),
// 		)
// 	}
// }

type TorrentPiece struct {
	checksum Hash
	// size      int
	// numBlocks int
	block []byte
}

func NewTorrentPiece(
	index int,
	info *TorrentInfo,
) (piece *TorrentPiece, keys iter.Seq[RequestMessage]) {
	size := info.pieceLength

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

type TorrentIndex struct {
	// requests Queue[RequestMessage]
	checksum Hash
	// length int
	requests []RequestMessage
	pieces   []*TorrentPiece
	send     chan *PieceMessage
	// failed []RequestMessage
	// done   chan PieceKey
}

func newTorrentIndexEmpty(info *TorrentInfo) (index *TorrentIndex) {
	// numBlocks := info.length/defaultBlockSize + 1
	index = &TorrentIndex{
		requests: make(
			[]RequestMessage,
			0,
			ceilDiv(info.length, defaultBlockSize),
		),
		pieces: make([]*TorrentPiece, len(info.pieces)),
		send:   make(chan *PieceMessage, 1),
		// completed: make([]PieceKey, 0, numBlocks),
	}

	return
}

func newTorrentIndex(info *TorrentInfo) (index *TorrentIndex) {
	index = newTorrentIndexEmpty(info)

	for num := range len(info.pieces) {
		piece, iter := NewTorrentPiece(num, info)

		index.pieces = append(index.pieces, piece)

		for msg := range iter {
			index.requests = append(index.requests, msg)
		}
	}

	return
}

func newTorrentIndexSingle(num int, info *TorrentInfo) (index *TorrentIndex) {
	index = newTorrentIndexEmpty(info)
	piece, iter := NewTorrentPiece(num, info)

	index.pieces = append(index.pieces, piece)

	for msg := range iter {
		index.requests = append(index.requests, msg)
	}

	return
}

// func downloadPiece(index, length int, peers TorrentPeers) (piece []byte) {
// peerPools := peers.withPiece(index)
// log.Printf("number of peer pools: %d", len(peerPools))
// handler := NewTorrentRequestHandler(defaultQueueSize)
// defer handler.Close()
// responses := make([]PiecePayload, 0, chunks)
// var wg sync.WaitGroup
// piece = make([]byte, length)

// // iter :=

// i := 0

// picked := peers.pick(index)
// pickedLen := len(picked)

// for msg := range IterRequestMessage(index, length, defaultBufferSize) {
// 	wg.Add(1)

// 	shift := i % pickedLen

// 	handler.send <- TorrentRequest{
// 		Peers: append(picked[shift:], picked[:shift]...),
// 		Msg:   msg,
// 	}

// 	i++
// }

// go receivePiece(handler.recv, &wg, &piece)

// log.Printf("waiting for %d\n", i)

// wg.Wait()

// log.Println("returning piece")

// 	return piece
// }

func CmdDownloadPiece(downloadPath, torrentPath string, index int) {
	torrent := ParseTorrentFile(torrentPath)

	send := make(chan PieceMessage, 1)

	_, err := allTorrentHandlers(torrent, send)
	if err != nil {
		panic(err)
	}
	// piece := downloadPiece(index, torrent.pieceLength, peers)
	// hash := sha1.Sum(piece)
	//
	//	if !bytes.Equal(hash[:], torrent.pieces[index][:]) {
	//		panic("piece hash differ")
	//	}
	//
	// log.Printf("writing %d bytes to %q", len(piece), downloadPath)
	// if err := os.WriteFile(downloadPath, piece, defaultFileMode); err != nil
	//
	//	{
	// 		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	//	}
	//
	//	if _, err := os.Stat(downloadPath); errors.Is(err, os.ErrNotExist) {
	// 		panic(fmt.Errorf("error writing file to %q: %w", downloadPath, err))
	//	}
	//
	// log.Println("file saved")
}

func CmdDownload(downloadPath, torrentPath string) {
}
