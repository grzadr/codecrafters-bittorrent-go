package internal

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
)

const (
	protocoLength         = 19
	protocolName          = "BitTorrent protocol"
	protocolReservedBytes = 8
	msgTypePos            = 4
	handshakeMsgLength    = 68
	bufferSize            = 16 * 1024
)

func intToBytes(n int, buf []byte) {
	binary.BigEndian.PutUint32(buf, uint32(n))
}

type MessageType int

const (
	Choke MessageType = iota
	Unchoke
	Interested
	NotInterested
	Have
	Bitfield
	Request
	Piece
	Cancel
)

type Message struct {
	msgType MessageType
	content []byte
}

func NewMessage(data []byte) (msg Message, read int) {
	size := int(binary.BigEndian.Uint32(data[:msgTypePos]))

	msg, read = Message{
		msgType: MessageType(data[msgTypePos]),
		content: data[msgTypePos:][:size-1],
	}, msgTypePos+size

	log.Printf("%+v [%d] from:\n%s", msg, size, hex.Dump(data))

	return
}

func NewInterestedMsg() Message {
	return Message{
		msgType: Interested,
		content: []byte{},
	}
}

func (m Message) encode() (msg []byte) {
	msg = make([]byte, msgTypePos+1+len(m.content))
	intToBytes(len(m.content)+1, msg)
	msg[msgTypePos] = byte(m.msgType)
	copy(msg[msgTypePos+1:], m.content[:])

	log.Printf("%+v:\n%s", m, hex.Dump(msg))

	return
}

type HandshakeResponse struct {
	id    string
	owned []byte
}

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

func (r HandshakeRequest) encode() []byte {
	message := make([]byte, handshakeMsgLength)
	message[0] = byte(len(r.protocol))
	offset := 1
	offset += copy(message[offset:], r.protocol)
	offset += protocolReservedBytes
	offset += copy(message[offset:], r.infoHash[:])
	copy(message[offset:], r.peerId)

	return message
}

func (req HandshakeRequest) make(
	conn *net.TCPConn,
) (response HandshakeResponse) {
	conn.Write(req.encode())

	response = HandshakeResponse{}
	resp := make([]byte, bufferSize)
	n := 0

	n, _ = conn.Read(resp)

	log.Println(hex.Dump(resp[:n]))

	response.id = hex.EncodeToString(resp[n-shaHashLength : n])

	n, _ = conn.Read(resp)

	if n == 0 {
		return response
	}

	bitfield, read := NewMessage(resp[:n])

	if read != n {
		panic(fmt.Sprintf("bitfield: read %d from %d long message", read, n))
	}

	response.owned = bitfield.content

	conn.Write(NewInterestedMsg().encode())

	n, _ = conn.Read(resp)

	unchoke, read := NewMessage(resp[:n])

	if read != n {
		panic(fmt.Sprintf("unchoke: read %d from %d long message", read, n))
	}

	if unchoke.msgType != Unchoke {
		panic(fmt.Sprintf("unchoke: unexpected msg type %+v", unchoke))
	}

	return response
}

func MakeHandshake(path, ip string) (id string) {
	torrent := ParseTorrentFile(path)

	conn, _ := net.DialTCP("tcp", nil, ParsePeerIP(ip).TcpAddr())

	response := NewHandshakeRequest(torrent.hash).make(conn)

	return fmt.Sprintf(
		"Peer ID: %s",
		response.id,
	)
}
