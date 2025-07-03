package internal

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
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

type TrackerRequest struct {
	AnnounceURL string
	InfoHash    []byte
	PeerID      string
	Port        int
	Uploaded    int
	Downloaded  int
	Left        int
}

func (req TrackerRequest) build() string {
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
func makeTrackerRequest(requestUrl string) (response Bencoded) {
	log.Println(requestUrl)

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

func DetectPeers(path string) (s string) {
	info := NewTorrentInfo(decodeTorrentFile(path).(BencodedMap))

	trackerReq := TrackerRequest{
		AnnounceURL: info.tracker,
		InfoHash:    info.hash[:],
		PeerID:      defaultClientId,    // 20-character client identifier
		Port:        defaultTrackerPort, // Standard BitTorrent port
		Uploaded:    0,                  // No data uploaded yet
		Downloaded:  0,                  // No data downloaded yet
		Left:        info.length,        // 1 MB remaining (example)
	}.build()

	response := makeTrackerRequest(trackerReq).(BencodedMap)

	peers := make([]string, 0)

	chunks := slices.Chunk(
		[]byte(response["peers"].(BencodedString)),
		defaultIpAddressSize,
	)

	for slice := range chunks {
		peers = append(
			peers,
			fmt.Sprintf(
				"%d.%d.%d.%d:%d",
				slice[0],
				slice[1],
				slice[2],
				slice[3],
				binary.BigEndian.Uint16(slice[4:]),
			),
		)
	}

	return strings.Join(peers, "\n")
}
