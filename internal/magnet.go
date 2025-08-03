package internal

import (
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func decodeHexString(hexStr string) ([]byte, error) {
	return hex.DecodeString(hexStr)
}

type MagnetLink struct {
	checksum   Hash
	filename   string
	trackerUrl string
}

func NewMagnetLink(linkStr string) MagnetLink {
	content, _ := strings.CutPrefix(linkStr, "magnet:?xt=urn:btih:")
	checksum, content, _ := strings.Cut(content, "&")
	filename, trackerUrl, _ := strings.Cut(content, "&")
	decodedUrl, _ := url.QueryUnescape(trackerUrl[3:])

	return MagnetLink{
		checksum:   newHash(checksum),
		filename:   filename[3:],
		trackerUrl: decodedUrl,
	}
}

// func (link MagnetLink) decodeUrl() string {
// 	decoded, _ := url.QueryUnescape(link.trackerUrl)

// 	return decoded
// }

func (link MagnetLink) prepareParams() url.Values {
	params := url.Values{}
	params.Set("peer_id", defaultClientId)
	params.Set("info_hash", string(link.checksum[:]))
	params.Set("left", "1")
	params.Set("port", strconv.Itoa(defaultTrackerPort))
	params.Set("downloaded", "0")
	params.Set("uploaded", "0")
	params.Set("compact", "1")

	return params
}

func (link MagnetLink) prepareQuery() *url.URL {
	u, err := url.Parse(link.trackerUrl)
	if err != nil {
		panic(fmt.Errorf("error parsing URL %q: %w", link.trackerUrl, err))
	}

	u.RawQuery = link.prepareParams().Encode()

	fmt.Printf("Query: %s\n", u)

	return u
}

func (link MagnetLink) sendRequest() BencodedMap {
	query := link.prepareQuery()
	client := &http.Client{
		Timeout: defaultRequestTimeout,
	}

	req, err := http.NewRequest(http.MethodGet, query.String(), nil)
	if err != nil {
		panic(fmt.Errorf("error creating request: %w", err))
	}

	resp, err := client.Do(req)
	if err != nil {
		panic(fmt.Errorf("error executing request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		panic(fmt.Errorf("unexpected status: %d", resp.StatusCode))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Errorf("error reading response: %w", err))
	}

	fmt.Printf("Response: %s\n", body)

	return NewBencoded(NewByteIteratorBytes(body)).(BencodedMap)
}

func (link MagnetLink) requestPeers() []*net.TCPAddr {
	response := link.sendRequest()

	// fmt.Println(hex.Dump(response["peers"].(BencodedString)))
	ipData := []byte(response["peers"].(BencodedString))

	peers := make([]*net.TCPAddr, 0, len(ipData)/defaultIpAddressSize)

	for data := range slices.Chunk(ipData, defaultIpAddressSize) {
		peers = append(peers, NewPeerIP(data))
	}

	return peers
}

func CmdMagnetParse(url string) string {
	link := NewMagnetLink(url)

	return fmt.Sprintf(
		"Tracker URL: %s\nInfo Hash: %s",
		link.trackerUrl,
		link.checksum,
	)
}

func CmdMagnetHandshake(linkStr string) string {
	link := NewMagnetLink(linkStr)

	peersIp := link.requestPeers()

	handshake := NewHandshakeRequestExt(link.checksum)

	peer, err := NewTorrentPeer(ParsePeerIP(peersIp[0].String()), handshake)
	if err != nil {
		panic(err)
	}
	defer peer.close()

	return fmt.Sprintf(
		"Peer ID: %s",
		peer.id,
	)
}
