package internal

import (
	"encoding/binary"
	"fmt"
	"io"
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
	defaultRequestTimeout = 30 * time.Second
	defaultClientId       = "GO-CLIENT-1234567890"
	defaultIpAddressSize  = 6
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

func CmdPeers(path string) (s string) {
	torrent := ParseTorrentFile(path)

	peers := make([]string, 0)

	for _, p := range NewDiscoverRequest(torrent).peers() {
		peers = append(peers, p.String())
	}

	return strings.Join(peers, "\n")
}
