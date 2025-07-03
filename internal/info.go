package internal

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
)

const shaHashLength = 20

func decodeTorrentFile(path string) Bencoded {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	iter := NewByteIteratorBytes(data)

	return NewBencoded(iter)
}

type PieceHashes [][]byte

func NewPieceHashes(pieces []byte) (hashes PieceHashes) {
	hashes = make(PieceHashes, len(pieces)/shaHashLength)

	for piece := range slices.Chunk([]byte(pieces), shaHashLength) {
		pieceHashes = append(pieceHashes, hex.EncodeToString(piece))
	}
}

type TorrentInfo struct {
	tracker     string
	hash        [shaHashLength]byte
	pieces      PieceHashes
	length      int
	pieceLength int
}

func NewTorrentInfo(torrent BencodedMap) *TorrentInfo {
	info := torrent["info"].(BencodedMap)

	return &TorrentInfo{
		tracker:     torrent["announce"].String(),
		hash:        sha1.Sum(info.Encode()),
		pieces:      NewPieceHashes(torrent["pieces"].(BencodedString)),
		length:      int(info["length"].(BencodedInteger)),
		pieceLength: int(info["piece length"].(BencodedInteger)),
	}
}

func ShowTorrentInfo(path string) string {
	parsed := decodeTorrentFile(path).(BencodedMap)
	info := parsed["info"].(BencodedMap)

	sum := sha1.Sum(info.Encode())

	infoHash := hex.EncodeToString(sum[:])

	pieceHashes := make([]string, 0)

	pieces := []byte(info["pieces"].(BencodedString))

	for piece := range slices.Chunk(pieces, shaHashLength) {
		pieceHashes = append(pieceHashes, hex.EncodeToString(piece))
	}

	return fmt.Sprintf(
		"Tracker URL: %s\nLength: %d\nInfo Hash: %s\nPiece Length: %d\nPiece Hashes:\n%s",
		parsed["announce"],
		info["length"],
		infoHash,
		info["piece length"],
		strings.Join(pieceHashes, "\n"),
	)
}
