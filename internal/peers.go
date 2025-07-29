package internal

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"iter"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTrackerPort    = 6681
	defaultRequestTimeout = 10 * time.Second
	defaultClientId       = "GO-CLIENT-1234567890"
	defaultIpAddressSize  = 6
	defaultPeerBuffer     = 128 * 1024
	byteSize              = 8
)

// type PeerIP net.TCPAddr

// type PeerIP struct {
// 	ip   net.IP
// 	port int
// }

func NewPeerIP(data []byte) (addr *net.TCPAddr) {
	addr = &net.TCPAddr{
		IP:   net.IPv4(data[0], data[1], data[2], data[3]),
		Port: int(binary.BigEndian.Uint16(data[4:])),
	}

	return
}

func ParsePeerIP(ip string) (addr *net.TCPAddr) {
	addrStr, portStr, _ := strings.Cut(ip, ":")
	port, _ := strconv.Atoi(portStr)

	addr = &net.TCPAddr{
		IP:   net.ParseIP(addrStr),
		Port: port,
	}

	return
}

// func (p PeerIP) String() string {
// 	return fmt.Sprintf("%s:%d", p.ip, p.port)
// }

// func (p PeerIP) TcpAddr() *net.TCPAddr {
// 	return &net.TCPAddr{
// 		IP:   p.ip,
// 		Port: p.port,
// 	}
// }

type DiscoverRequest struct {
	AnnounceURL string
	InfoHash    []byte
	PeerID      string
	Port        int
	Uploaded    int
	Downloaded  int
	Left        int
}

func NewDiscoverRequest(torrent *TorrentInfo) (req *DiscoverRequest) {
	return &DiscoverRequest{
		AnnounceURL: torrent.tracker,
		InfoHash:    torrent.hash[:],
		PeerID:      defaultClientId,    // 20-character client identifier
		Port:        defaultTrackerPort, // Standard BitTorrent port
		Uploaded:    0,                  // No data uploaded yet
		Downloaded:  0,                  // No data downloaded yet
		Left:        torrent.length,     // 1 MB remaining (example)
	}
}

func (req DiscoverRequest) build() string {
	baseURL, err := url.Parse(req.AnnounceURL)
	if err != nil {
		panic(fmt.Errorf("invalid announce URL: %w", err))
	}

	params := url.Values{}
	params.Set("info_hash", string(req.InfoHash))
	params.Set("peer_id", req.PeerID)
	params.Set("port", strconv.Itoa(req.Port))
	params.Set("uploaded", strconv.Itoa(req.Uploaded))
	params.Set("downloaded", strconv.Itoa(req.Downloaded))
	params.Set("left", strconv.Itoa(req.Left))
	params.Set("compact", "1")

	baseURL.RawQuery = params.Encode()

	return baseURL.String()
}

// makeTrackerRequest performs the actual HTTP GET request to the tracker
// It includes proper timeout handling and error management.
func (req DiscoverRequest) make() (response BencodedMap) {
	requestUrl := req.build()

	client := &http.Client{
		Timeout: defaultRequestTimeout,
	}

	// Make the GET request
	resp, err := client.Get(requestUrl)
	if err != nil {
		panic(fmt.Errorf("HTTP request failed: %w", err))
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		panic(fmt.Errorf(
			"tracker responded with status %d: %s",
			resp.StatusCode,
			resp.Status,
		))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Errorf("failed to read response body: %w", err))
	}

	response = NewBencoded(NewByteIteratorBytes(data)).(BencodedMap)

	return response
}

func (req DiscoverRequest) peers() (p []*net.TCPAddr) {
	peersBytes := []byte(req.make()["peers"].(BencodedString))
	p = make([]*net.TCPAddr, 0, len(peersBytes)/defaultIpAddressSize)

	for c := range slices.Chunk(peersBytes, defaultIpAddressSize) {
		p = append(p, NewPeerIP(c))
	}

	return
}

type TorrentPeer struct {
	id     string
	owned  []byte
	conn   *net.TCPConn
	addr   *net.TCPAddr
	reader *bufio.Reader
}

func NewTorrentPeer(
	addr *net.TCPAddr,
	handshake []byte,
) (peer *TorrentPeer, err error) {
	peer = &TorrentPeer{
		addr: addr,
	}

	if err := peer.dial(); err != nil {
		return peer, fmt.Errorf(
			"failed to establish connection with %q: %w",
			peer.addr,
			err,
		)
	}

	log.Println("conn established")

	if err = peer.handshake(handshake); err != nil {
		return peer, fmt.Errorf("error during handshake: %w", err)
	}

	log.Println("handshake performed")

	return peer, err
}

func (peer *TorrentPeer) write(msg []byte) error {
	totalWritten := 0

	for totalWritten < len(msg) {
		n, err := peer.conn.Write(msg[totalWritten:])
		if err != nil {
			return fmt.Errorf(
				"write failed after %d bytes: %w",
				totalWritten,
				err,
			)
		}

		if n == 0 {
			return fmt.Errorf("zero bytes written without error")
		}

		totalWritten += n
	}

	return nil
}

func (peer *TorrentPeer) dial() (err error) {
	peer.conn, err = net.DialTCP("tcp", nil, peer.addr)
	if err != nil {
		err = fmt.Errorf("error making connection: %s", err)

		return
	}

	peer.reader = bufio.NewReaderSize(peer.conn, defaultPeerBuffer)

	return
}

func (peer *TorrentPeer) interested() error {
	if err := peer.write(NewInterestedMsg().encode()); err != nil {
		return fmt.Errorf("error sending interested: %w", err)
	}

	found := false

	for msg := range NewMessage(peer.reader) {
		if msg.Err != nil {
			return fmt.Errorf("failed to read unchoke: %w", msg.Err)
		}

		if msg.Type == Unchoke {
			// log.Println("received unchoke")
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

func (peer *TorrentPeer) handshake(
	handshake []byte,
) error {
	if err := peer.write(handshake); err != nil {
		return err
	}

	next, stop := iter.Pull(NewMessage(peer.reader))
	defer stop()

	response, ok := next()

	if !ok || response.Err != nil {
		return fmt.Errorf(
			"failed to fetch initial handshake response: %w",
			response.Err,
		)
	}

	peer.id = hex.EncodeToString(response.content[response.Size-shaHashLength:])

	response, ok = next()

	if !ok {
		return nil
	}

	if response.Err != nil {
		return fmt.Errorf("error reading bitfield: %w", response.Err)
	}

	peer.owned = response.content

	err := peer.interested()

	return err
}

func (peer *TorrentPeer) close() {
	peer.conn.Close()
	peer.conn = nil
	peer.reader = nil
}

func (peer *TorrentPeer) hasPiece(num int) bool {
	pos := num / byteSize
	shift := byteSize - (num % byteSize) - 1

	log.Printf("%08b pos: %d shift %d\n", peer.owned, pos, shift)

	return 0x01&(peer.owned[pos]>>shift) == 1
}

func (peer *TorrentPeer) setTimeout(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	if err := peer.conn.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	return nil
}

func (peer *TorrentPeer) resetTimeout() {
	peer.conn.SetReadDeadline(time.Time{})
}

func allTorrentPeers(info *TorrentInfo) (peers []*TorrentPeer, err error) {
	addresses := NewDiscoverRequest(info).peers()
	peers = make([]*TorrentPeer, len(addresses))
	handshake := NewHandshakeRequest(info.hash)

	for i, addr := range addresses {
		if peers[i], err = NewTorrentPeer(addr, handshake); err != nil {
			break
		}
	}

	return
}

func CmdHandshake(path, ip string) (id string) {
	torrent := ParseTorrentFile(path)

	handshake := NewHandshakeRequest(torrent.hash)

	peer, err := NewTorrentPeer(ParsePeerIP(ip), handshake)
	if err != nil {
		panic(err)
	}

	defer peer.close()

	return fmt.Sprintf(
		"Peer ID: %s",
		peer.id,
	)
}

func CmdPeers(path string) (s string) {
	torrent := ParseTorrentFile(path)

	peers := make([]string, 0)

	for _, p := range NewDiscoverRequest(torrent).peers() {
		peers = append(peers, p.String())
	}

	return strings.Join(peers, "\n")
}
