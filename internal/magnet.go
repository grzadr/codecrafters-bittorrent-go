package internal

import (
	"encoding/hex"
	"fmt"
	"net/url"
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

	return MagnetLink{
		checksum:   newHash(checksum),
		filename:   filename[3:],
		trackerUrl: trackerUrl[3:],
	}
}

func (link MagnetLink) decodeUrl() string {
	decoded, _ := url.QueryUnescape(link.trackerUrl)

	return decoded
}

func CmdMagnetParse(url string) string {
	link := NewMagnetLink(url)

	return fmt.Sprintf(
		"Tracker URL: %s\nInfo Hash: %s",
		link.decodeUrl(),
		link.checksum,
	)
}
