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
	protocoLength         = 19
	protocolName          = "BitTorrent protocol"
	protocolReservedBytes = 8
	msgTypePos            = 4
	handshakeMsgLength    = 68
	requestFieldsNum      = 3
	defaultBufferSize     = 16 * 1024
	byteSize              = 8
	int32Size             = 4
	defaultNumConnections = 1
)

func intToBytes(n int, buf []byte) {
	binary.BigEndian.PutUint32(buf, uint32(n))
}

func bytesToInt(data []byte) int {
	return int(binary.BigEndian.Uint32(data))
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
	msgType := MessageType(data[msgTypePos])

	log.Println("msgType", msgType.String())

	if msgType == Piece {
		log.Println(size)
		log.Println(hex.Dump(data[:20]))
	}

	msg, read = Message{
		msgType: msgType,
		content: data[msgTypePos+1:][:size-1],
	}, msgTypePos+size

	return
}

func ReadNewMessage(
	conn *net.TCPConn,
	msgType MessageType,
) (msg Message, err error) {
	resp := make([]byte, defaultBufferSize)

	n, err := conn.Read(resp)

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
	content := make([]byte, int32Size*requestFieldsNum)

	intToBytes(index, content[0:])
	intToBytes(begin, content[int32Size:])
	intToBytes(length, content[int32Size*2:])

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
	contentLength := len(m.content) + 1
	msg = make([]byte, msgTypePos+contentLength)
	intToBytes(contentLength, msg)
	msg[msgTypePos] = byte(m.msgType)
	copy(msg[msgTypePos+1:], m.content[:])

	return
}

type PeerConnectionPool struct {
	pool chan *net.TCPConn
	addr *net.TCPAddr
}

func NewPeerConnectionPool(
	addr *net.TCPAddr,
	num int,
	handshake []byte,
) (id string, owned []byte, pool *PeerConnectionPool) {
	pool = &PeerConnectionPool{pool: make(chan *net.TCPConn, num), addr: addr}

	for range num {
		conn, err := net.DialTCP("tcp", nil, pool.addr)
		if err != nil {
			panic(fmt.Sprintf("error making connection: %s", err))
		}

		pool.pool <- conn
	}

	id, owned, err := pool.performHandshake(handshake)
	if err != nil {
		panic(fmt.Sprintf("error performing handshake: %s", err))
	}

	// for range num - 1 {
	// 	pool.performHandshake(handshake)
	// }

	return
}

func (p *PeerConnectionPool) acquire() (conn *net.TCPConn) {
	conn = <-p.pool

	return
}

func (p *PeerConnectionPool) release(conn *net.TCPConn) {
	p.pool <- conn
}

func sendInterested(conn *net.TCPConn) (err error) {
	if _, err = conn.Write(NewInterestedMsg().encode()); err != nil {
		return err
	}

	if _, err = ReadNewMessage(conn, Unchoke); err != nil {
		err = fmt.Errorf("error read unchoke: %w", err)
	}

	return
}

func (p *PeerConnectionPool) performHandshake(
	handshake []byte,
) (id string, owned []byte, err error) {
	conn := p.acquire()
	defer p.release(conn)

	if _, err = conn.Write(handshake); err != nil {
		return id, owned, err
	}

	resp := make([]byte, defaultBufferSize)
	n := 0

	if n, err = conn.Read(resp); err != nil {
		return id, owned, err
	}

	id = hex.EncodeToString(resp[n-shaHashLength : n])

	bitfield, err := ReadNewMessage(conn, Bitfield)
	if err == io.EOF {
		err = nil

		return id, owned, err
	} else if err != nil {
		err = fmt.Errorf("error read bitfield: %w", err)

		return id, owned, err
	}

	owned = bitfield.content

	log.Println("owned: %+v", owned)

	err = sendInterested(conn)

	return id, owned, err
}

func (p *PeerConnectionPool) close() {
	defer close(p.pool)

	for {
		select {
		case conn := <-p.pool:
			conn.Close()
		default:
			return
		}
	}
}

type PeerConnection struct {
	pool  *PeerConnectionPool
	id    string
	owned []byte
	Err   error
}

func NewPeerConnection(
	addr *net.TCPAddr,
	handshake []byte,
	num int,
) (peer *PeerConnection) {
	peer = &PeerConnection{}
	peer.id, peer.owned, peer.pool = NewPeerConnectionPool(
		addr,
		num,
		handshake,
	)

	return
}

// func (p *PeerConnection) write(content []byte) {
// 	_, p.Err = p.conn.Write(content)
// 	if p.Err != nil {
// 		p.Err = fmt.Errorf(
// 			"error sending request: %w\n%s",
// 			p.Err,
// 			hex.Dump(content),
// 		)
// 	}
// }

// func (p *PeerConnection) read() (resp []byte, n int) {
// 	resp = make([]byte, defaultBufferSize)

// 	n, err := p.conn.Read(resp)
// 	if err != nil {
// 		p.Err = fmt.Errorf("error reading from conn: %w", err)
// 	}

// 	resp = resp[:n]

// 	return
// }

func (p *PeerConnection) close() {
	p.pool.close()
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

// func (req HandshakeRequest) make(
// 	addr *net.TCPAddr,
// ) (peer *PeerConnection) {
// 	peer = NewPeerConnection(addr)

// 	if peer.Err != nil {
// 		return peer
// 	}

// 	peer.write(req.encode())

// 	if peer.Err != nil {
// 		return peer
// 	}

// 	resp, n := peer.read()

// 	if peer.Err != nil {
// 		return peer
// 	}

// 	peer.id = hex.EncodeToString(resp[n-shaHashLength : n])

// 	bitfield, err := ReadNewMessage(peer.conn, Bitfield)
// 	if err == io.EOF {
// 		return peer
// 	} else if err != nil {
// 		peer.Err = fmt.Errorf("error read bitfield: %w", err)

// 		return peer
// 	}

// 	peer.owned = bitfield.content

// 	peer.write(NewInterestedMsg().encode())

// 	if peer.Err != nil {
// 		return peer
// 	}

// 	_, err = ReadNewMessage(peer.conn, Unchoke)
// 	if err != nil {
// 		peer.Err = fmt.Errorf("error read unchoke: %w", err)
// 	}

// 	return peer
// }

type TorrentPeers []*PeerConnection

func NewTorrentPeers(
	hash Hash,
	addr []*net.TCPAddr,
) (peers TorrentPeers) {
	peers = make(TorrentPeers, len(addr))

	var wg sync.WaitGroup

	wg.Add(len(addr))

	handshake := NewHandshakeRequest(hash).encode()

	for i, a := range addr {
		go func(i int, a *net.TCPAddr) {
			defer wg.Done()

			peers[i] = NewPeerConnection(a, handshake, defaultNumConnections)
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

func NewTorrentPeersFromInfo(info *TorrentInfo) (peers TorrentPeers) {
	addr := NewDiscoverRequest(info).peers()

	return NewTorrentPeers(info.hash, addr)
}

func (t *TorrentPeers) withPiece(index int) (conns []*PeerConnectionPool) {
	conns = make([]*PeerConnectionPool, 0, len(*t))

	pos := index / byteSize
	shift := byteSize - (index % byteSize) - 1

	for _, peer := range *t {
		if 0x01&(peer.owned[pos]>>shift) == 1 {
			conns = append(conns, peer.pool)
		}
	}

	return conns
}

func (t *TorrentPeers) close() {
	for _, peer := range *t {
		peer.close()
	}
}

func CmdHandshake(path, ip string) (id string) {
	torrent := ParseTorrentFile(path)

	handshake := NewHandshakeRequest(torrent.hash).encode()

	peer := NewPeerConnection(ParsePeerIP(ip), handshake, 1)
	if peer.Err != nil {
		panic(peer.Err)
	}

	defer peer.close()

	return fmt.Sprintf(
		"Peer ID: %s",
		peer.id,
	)
}
