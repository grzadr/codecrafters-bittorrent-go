package internal

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"net"
)

const (
	protocoLength         = 19
	protocolName          = "BitTorrent protocol"
	protocolReservedBytes = 8
	handshakeMsgLength    = 68
	responseBufferSize    = 16 * 1024
)

type HandshakeRequest struct {
	protocol string
	infoHash Hash
	peerId   string
}

func NewHandshakeRequest(hash Hash) (req *HandshakeRequest) {
	return &HandshakeRequest{
		protocol: protocolName,
		infoHash: hash,
		peerId:   defaultClientId,
	}
}

func (r HandshakeRequest) encode() (message []byte) {
	message = make([]byte, 0, handshakeMsgLength)

	message = append(message, byte(len(r.protocol)))
	message = append(message, []byte(r.protocol)...)
	message = append(message, bytes.Repeat([]byte{0}, protocolReservedBytes)...)
	message = append(message, r.infoHash[:]...)
	message = append(message, []byte(defaultClientId)...)

	return
}

func MakeHandshake(path, ip string) (id string) {
	torrent := ParseTorrentFile(path)

	conn, _ := net.DialTCP("tcp", nil, ParsePeerIP(ip).TcpAddr())

	request := NewHandshakeRequest(torrent.hash)

	conn.Write(request.encode())

	response := make([]byte, responseBufferSize)

	n, _ := conn.Read(response)

	response = response[:n]

	log.Println(hex.Dump(response))

	return fmt.Sprintf(
		"Peer ID: %s",
		hex.EncodeToString(response[n-shaHashLength:]),
	)
}
