package internal

import "iter"

type PieceSpec struct {
	index int
	begin int
	block int
}

func IterPieceSpecs(index, size, chunk int) iter.Seq[PieceSpec] {
	return func(yield func(PieceSpec) bool) {
		begin := 0

		for block := range size / chunk {
			if !yield(PieceSpec{index: index, begin: begin, block: block}) {
				return
			}

			begin += block
		}

		if left := size % chunk; left > 0 {
			if !yield(PieceSpec{index: index, begin: begin, block: left}) {
				return
			}
		}
	}
}

func CmdDownloadPiece(downloadPath, torrentPath, pieceIndex string) {
	torrent := ParseTorrentFile(torrentPath)

	addr := NewDiscoverRequest(torrent).peers()

	// NewHandshakeRequest(torrent.hash).make(addr)

	peers := NewTorrentPeers(torrent.hash, addr)
	// response := make([]byte, bufferSize)
	// message := make([]byte, bufferSize)
	// n := 0
	// n, _ = conn.Read(response)
	// log.Println(hex.Dump(response[:n]))
	// intToBytesOptimized(1, message)
	// message[4] = 2
	// conn.Write(message)
	// n, _ = conn.Read(response)
	// log.Println(hex.Dump(response[:n]))
}

func CmdDownload(downloadPath, torrentPath string) {
}

// TODO Plan
// 1. Connect to all peers and receive pieces availability
// 2. Organize pieces availability in slice of []*conn
// 3. Divide pieces into slices of 16k and prepare requests
// 4. Send requests to multiple peers
// 5. Prepare an empty file
// 5. Collect data for each piece, calc checksum and save to file
