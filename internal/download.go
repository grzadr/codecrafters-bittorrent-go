package internal

import (
	"encoding/binary"
	"net"
)

func intToBytesOptimized(n int32, buf []byte) {
	binary.BigEndian.PutUint32(buf, uint32(n))
}

func Download(downloadPath, torrentPath string) {
}

func DownloadPiece(downloadPath, torrentPath, pieceIndex string) {
	torrent := ParseTorrentFile(torrentPath)

	ip := NewDiscoverRequest(torrent).peers()[0]

	conn, _ := net.DialTCP("tcp", nil, ip.TcpAddr())

	NewHandshakeRequest(torrent.hash).make(conn)
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

// TODO Plan
// 1. Connect to all peers and receive pieces availability
// 2. Organize pieces availability in slice of []*conn
// 3. Divide pieces into slices of 16k and prepare requests
// 4. Send requests to multiple peers
// 5. Prepare an empty file
// 5. Collect data for each piece, calc checksum and save to file
