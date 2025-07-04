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
