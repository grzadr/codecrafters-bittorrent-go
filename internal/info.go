package internal

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
)

func decodeTorrentFile(path string) Bencoded {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	iter := NewByteIteratorBytes(data)

	return NewBencoded(iter)
}

func TorrentInfo(path string) string {
	parsed := decodeTorrentFile(path).(BencodedMap)
	info := parsed["info"].(BencodedMap)

	sum := sha1.Sum(info.Encode())

	infoHash := hex.EncodeToString(sum[:])

	pieceHashes := make([]string, 0)

	pieces := []byte(info["pieces"].(BencodedString))

	for piece := range slices.Chunk(pieces, 20) {
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
