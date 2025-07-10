package internal

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

const (
	protocoLength           = 19
	protocolName            = "BitTorrent protocol"
	protocolReservedBytes   = 8
	msgTypePos              = 4
	handshakeMsgLength      = 68
	requestMsgContentLength = 12
	bufferSize              = 16 * 1024
	byteSize                = 8
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

var messageTypeNames = [...]string{
	"Choke",
	"Unchoke",
	"Interested",
	"NotInterested",
	"Have",
	"Bitfield",
	"Request",
	"Piece",
	"Cancel",
}

func (m MessageType) String() string {
	if m < 0 || int(m) >= len(messageTypeNames) {
		panic(fmt.Sprintf("MessageType(%d)", int(m)))
	}

	return messageTypeNames[m]
}

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

func ReadNewMessage(
	conn *net.TCPConn,
	msgType MessageType,
) (msg Message, err error) {
	resp := make([]byte, bufferSize)

	n, err := conn.Read(resp)

	log.Println(hex.Dump(resp[:n]))

	if n == 0 || err != nil {
		return
	}

	msg, read := NewMessage(resp[:n])

	if read != n {
		panic(fmt.Sprintf("%s: read %d from %d long message", msgType, read, n))
	}

	if msg.msgType != msgType {
		panic(fmt.Sprintf("unexpected msg type %+v", msg))
	}

	return
}

func NewRequestMessage(index, begin, length int) Message {
	content := make([]byte, requestMsgContentLength)

	intToBytes(index, content)
	intToBytes(begin, content[4:])
	intToBytes(length, content[8:])

	return Message{
		msgType: Request,
		content: content,
	}
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

	return
}

type PeerConnection struct {
	conn  *net.TCPConn
	id    string
	owned []byte
	Err   error
}

func NewPeerConnection(addr *net.TCPAddr) (peer *PeerConnection) {
	peer = &PeerConnection{}
	conn, err := net.DialTCP("tcp", nil, addr)
	peer.conn = conn

	if err != nil {
		peer.Err = fmt.Errorf("error connecting to %q: %w", addr, err)
	}

	return
}

func (p *PeerConnection) write(content []byte) {
	_, p.Err = p.conn.Write(content)
	if p.Err != nil {
		p.Err = fmt.Errorf(
			"error sending request: %w\n%s",
			p.Err,
			hex.Dump(content),
		)
	}

	return
}

func (p *PeerConnection) read() (resp []byte, n int) {
	resp = make([]byte, bufferSize)

	n, err := p.conn.Read(resp)
	if err != nil {
		p.Err = fmt.Errorf("error reading from conn: %w", err)
	}

	resp = resp[:n]

	return
}

func (p *PeerConnection) close() {
	p.close()
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
	addr *net.TCPAddr,
) (peer *PeerConnection) {
	peer = NewPeerConnection(addr)

	if peer.Err != nil {
		return peer
	}

	peer.write(req.encode())

	if peer.Err != nil {
		return peer
	}

	resp, n := peer.read()

	if peer.Err != nil {
		return peer
	}

	log.Println(hex.Dump(resp[:n]))

	peer.id = hex.EncodeToString(resp[n-shaHashLength : n])

	bitfield, err := ReadNewMessage(peer.conn, Bitfield)
	if err == io.EOF {
		return peer
	} else if err != nil {
		peer.Err = fmt.Errorf("error read bitfield: %w", err)

		return peer
	}

	peer.owned = bitfield.content

	peer.write(NewInterestedMsg().encode())

	if peer.Err != nil {
		return peer
	}

	_, err = ReadNewMessage(peer.conn, Unchoke)
	if err != nil {
		peer.Err = fmt.Errorf("error read unchoke: %w", err)
	}

	return peer
}

type TorrentPeers []*PeerConnection

func NewTorrentPeers(
	hash Hash,
	addr []*net.TCPAddr,
) (peers TorrentPeers) {
	peers = make(TorrentPeers, len(addr))

	var wg sync.WaitGroup

	wg.Add(len(addr))

	for i, a := range addr {
		go func(i int, a *net.TCPAddr) {
			defer wg.Done()

			peers[i] = NewHandshakeRequest(hash).make(a)
		}(i, a)
	}

	wg.Wait()

	for _, p := range peers {
		if p.Err != nil {
			panic(p.Err)
		}
	}

	return peers
}

func (t *TorrentPeers) withPiece(index int) (conns []*net.TCPConn) {
	conns = make([]*net.TCPConn, 0, len(*t))

	pos := index / byteSize
	shift := byteSize - (index % byteSize) - 1

	for _, peer := range *t {
		if peer.owned[pos]&(1<<shift) == 1 {
			conns = append(conns, peer.conn)
		}
	}

	return
}

func (t *TorrentPeers) close() {
	for _, peer := range *t {
		peer.close()
	}
}

func CmdHandshake(path, ip string) (id string) {
	torrent := ParseTorrentFile(path)

	peer := NewHandshakeRequest(torrent.hash).make(ParsePeerIP(ip))
	if peer.Err != nil {
		panic(peer.Err)
	}

	defer peer.close()

	return fmt.Sprintf(
		"Peer ID: %s",
		peer.id,
	)
}
