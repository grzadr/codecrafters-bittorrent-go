package internal

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"iter"
	"log"
	"net"
	"sync"
	"time"
)

const (
	protocoLength         = 19
	protocolName          = "BitTorrent protocol"
	handshakePrefix       = "\x13Bit"
	handshakePrefixLength = len(handshakePrefix)
	protocolReservedBytes = 8
	msgLengthBytes        = 4
	handshakeMsgLength    = 68
	requestFieldsNum      = 3
	defaultBufferSize     = 16 * 1024
	byteSize              = 8
	int32Size             = 4
	defaultNumConnections = 1
	timeout               = 5 * time.Second
)

func ReadNewMessage(conn *net.TCPConn) iter.Seq[Message] {
	reader := bufio.NewReaderSize(conn, defaultByteBuffer)

	return NewMessage(reader)
}

type RequestMessage struct {
	index int
	begin int
	block int
}

func (m RequestMessage) encode() (msg []byte) {
	contentLength := int32Size*requestFieldsNum + 1
	msg = make([]byte, msgLengthBytes+contentLength)
	intToBytes(contentLength, msg)
	msg[msgLengthBytes] = byte(Request)

	intToBytes(m.index, msg[msgLengthBytes+1:])
	intToBytes(m.begin, msg[msgLengthBytes+int32Size+1:])
	intToBytes(m.block, msg[msgLengthBytes+int32Size*2+1:])

	return
}

// func NewRequestMessage(index, begin, length int) RequestMessage {
// 	content := make([]byte, int32Size*requestFieldsNum)

// 	intToBytes(index, content[0:])
// 	intToBytes(begin, content[int32Size:])
// 	intToBytes(length, content[int32Size*2:])

// 	return Message{
// 		msgType: Request,
// 		content: content,
// 	}
// }

func NewInterestedMsg() Message {
	return Message{
		Type:    Interested,
		content: []byte{},
	}
}

func (m Message) encode() (msg []byte) {
	contentLength := len(m.content) + 1
	msg = make([]byte, msgLengthBytes+contentLength)
	intToBytes(contentLength, msg)
	msg[msgLengthBytes] = byte(m.Type)
	copy(msg[msgLengthBytes+1:], m.content[:])

	return
}

type PeerConnectionPool struct {
	// pool chan *net.TCPConn
	conn *net.TCPConn
	lock sync.Mutex
	addr *net.TCPAddr
}

func NewPeerConnectionPool(
	addr *net.TCPAddr,
	num int,
	handshake []byte,
) (id string, owned []byte, pool *PeerConnectionPool) {
	var err error

	pool = &PeerConnectionPool{addr: addr}

	pool.conn, err = net.DialTCP("tcp", nil, pool.addr)
	if err != nil {
		panic(fmt.Sprintf("error making connection: %s", err))
	}

	id, owned, err = pool.performHandshake(handshake)

	log.Println("handshake performed")

	if err != nil {
		panic(fmt.Sprintf("error performing handshake: %s", err))
	}

	// for range num - 1 {
	// 	pool.performHandshake(handshake)
	// }

	return
}

func (p *PeerConnectionPool) acquire() *net.TCPConn {
	p.lock.Lock()

	return p.conn
}

func (p *PeerConnectionPool) release() {
	p.lock.Unlock()
}

func sendEncoded(conn *net.TCPConn, msg []byte) error {
	if n, err := conn.Write(msg); err != nil {
		return fmt.Errorf("error writing request: %w", err)
	} else if n != len(msg) {
		return fmt.Errorf("error writing %d bytes: wrote %d", len(msg), n)
	}

	return nil
}

func sendInterested(conn *net.TCPConn) error {
	if err := sendEncoded(conn, NewInterestedMsg().encode()); err != nil {
		return fmt.Errorf("error sending interested: %w", err)
	}

	found := false

	for msg := range ReadNewMessage(conn) {
		if msg.Err != nil {
			return fmt.Errorf("failed to read unchoke: %w", msg.Err)
		}

		if msg.Type == Unchoke {
			log.Println("received unchoke")

			found = true

			break
		}
	}

	if !found {
		return fmt.Errorf(
			"error sending interested: no unchoke message detected",
		)
	}

	return nil
}

func (p *PeerConnectionPool) performHandshake(
	handshake []byte,
) (id string, owned []byte, err error) {
	conn := p.acquire()
	defer p.release()

	if _, err = conn.Write(handshake); err != nil {
		return id, owned, fmt.Errorf("error sending handshake: %w", err)
	}

	// for msg := range ReadNewMessage(conn) {
	// 	log.Println(msg)
	// }

	next, stop := iter.Pull(ReadNewMessage(conn))
	defer stop()

	// conn.SetReadDeadline(time.Now().Add(timeout))

	response, ok := next()

	if !ok || response.Err != nil {
		return id, owned, response.Err
	}

	id = hex.EncodeToString(response.content[response.Size-shaHashLength:])

	// for msg := range ReadNewMessage(conn) {
	// 	if msg.Err != nil {
	// 		err = fmt.Errorf("failed to read handshake response: %w", msg.Err)

	// 		return id, owned, err
	// 	}
	// }

	// conn.SetReadDeadline(time.Now().Add(timeout))

	response, ok = next()

	if !ok {
		return id, owned, err
	} else if response.Err != nil {
		return id, owned, fmt.Errorf("error reading bitfield: %w", response.Err)
	}

	owned = response.content

	// resp := make([]byte, defaultBufferSize)
	// n := 0

	// if n, err = conn.Read(resp); err != nil {
	// 	return id, owned, err
	// }

	// log.Println(hex.Dump(resp[:n]))

	// bitfield, err := ReadNewMessage(conn, Bitfield)
	// if err == io.EOF {
	// 	err = nil

	// 	return id, owned, err
	// } else if err != nil {
	// 	err = fmt.Errorf("error read bitfield: %w", err)

	// 	return id, owned, err
	// }

	// owned = bitfield.content

	log.Println("owned: %+v", owned)

	// conn.SetReadDeadline(time.Now().Add(timeout))

	err = sendInterested(conn)

	return id, owned, err
}

func (p *PeerConnectionPool) close() {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.conn.Close()
	p.conn = nil
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

type TorrentPeers struct {
	// conn    chan *PeerConnection
	peers   []*PeerConnection
	numConn int
}

func NewTorrentPeers(
	hash Hash,
	addr []*net.TCPAddr,
) (peers TorrentPeers) {
	peers = TorrentPeers{
		peers:   make([]*PeerConnection, len(addr)),
		numConn: len(addr),
	}

	var wg sync.WaitGroup

	wg.Add(len(addr))

	handshake := NewHandshakeRequest(hash).encode()

	for i, a := range addr {
		go func(i int, a *net.TCPAddr) {
			defer wg.Done()

			conn := NewPeerConnection(a, handshake, defaultNumConnections)
			if conn.Err != nil {
				log.Println("conn panick", conn.Err)
				panic(conn.Err)
			}

			peers.peers[i] = conn
		}(i, a)
	}

	wg.Wait()

	return peers
}

func NewTorrentPeersFromInfo(info *TorrentInfo) (peers TorrentPeers) {
	addr := NewDiscoverRequest(info).peers()

	return NewTorrentPeers(info.hash, addr)
}

// func (p *TorrentPeers) acquire(index int) (conn *PeerConnection) {
// 	pos := index / byteSize
// 	shift := byteSize - (index % byteSize) - 1

// 	for range p.numConn {
// 		conn = <-p.conn
// 		if 0x01&(conn.owned[pos]>>shift) == 1 {
// 			return conn
// 		}
// 	}

// 	panic(fmt.Sprintf("no peers with requested index %d", index))
// }

func (p *TorrentPeers) pick(index int) (peers []*PeerConnection) {
	pos := index / byteSize
	shift := byteSize - (index % byteSize) - 1
	peers = make([]*PeerConnection, len(p.peers))

	for i, conn := range p.peers {
		if 0x01&(conn.owned[pos]>>shift) == 1 {
			peers[i] = conn
		}
	}

	return peers
}

// func (p *TorrentPeers) release(conn *PeerConnection) {
// 	p.conn <- conn
// }

// func (t *TorrentPeers) withPiece(index int) (conns []*PeerConnectionPool) {
// 	conns = make([]*PeerConnectionPool, 0, len(*t))

// 	pos := index / byteSize
// 	shift := byteSize - (index % byteSize) - 1

// 	for range  {
// 		conn := peers.acquire()
// 		if conn.Err != nil {
// 			panic(conn.Err)
// 		}

// 		peers.release(conn)
// 	}

// 	for _, peer := range *t {
// 		if 0x01&(peer.owned[pos]>>shift) == 1 {
// 			conns = append(conns, peer.pool)
// 		}
// 	}

// 	return conns
// }

func (t *TorrentPeers) close() {
	for _, peer := range t.peers {
		peer.pool.close()
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
