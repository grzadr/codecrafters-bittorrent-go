package internal

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
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

	return fmt.Sprintf(
		"Tracker URL: %s\nLength: %d\nInfo hash: %s",
		parsed["announce"],
		info["length"],
		infoHash,
	)
}
