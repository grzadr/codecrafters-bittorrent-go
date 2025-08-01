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

type Hash [shaHashLength]byte

func newHash(hexStr string) (hash Hash) {
	decoded, _ := hex.DecodeString(hexStr)

	return Hash(decoded[:shaHashLength])
}

func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

type PieceHashes []Hash

func NewPieceHashes(pieces []byte) (hashes PieceHashes) {
	hashes = make(PieceHashes, 0, len(pieces)/shaHashLength)
	for piece := range slices.Chunk(pieces, shaHashLength) {
		hashes = append(hashes, Hash(piece))
	}

	return
}

func (h PieceHashes) ToString() (s []string) {
	s = make([]string, len(h))
	for i, hash := range h {
		s[i] = hash.String()
	}

	return
}

type TorrentInfo struct {
	tracker     string
	hash        Hash
	pieces      PieceHashes
	length      int
	pieceLength int
}

func NewTorrentInfo(torrent BencodedMap) *TorrentInfo {
	info := torrent["info"].(BencodedMap)

	return &TorrentInfo{
		tracker:     torrent["announce"].String(),
		hash:        sha1.Sum(info.Encode()),
		pieces:      NewPieceHashes(info["pieces"].(BencodedString)),
		length:      int(info["length"].(BencodedInteger)),
		pieceLength: int(info["piece length"].(BencodedInteger)),
	}
}

func ParseTorrentFile(path string) *TorrentInfo {
	return NewTorrentInfo(decodeTorrentFile(path).(BencodedMap))
}

func CmdInfo(path string) string {
	parsed := NewTorrentInfo(decodeTorrentFile(path).(BencodedMap))

	return fmt.Sprintf(
		"Tracker URL: %s\nLength: %d\nInfo Hash: %s\nPiece Length: %d\nPiece Hashes:\n%s",
		parsed.tracker,
		parsed.length,
		parsed.hash,
		parsed.pieceLength,
		strings.Join(parsed.pieces.ToString(), "\n"),
	)
}
