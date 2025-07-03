package main

import (
	"fmt"
	"log"
	"os"

	"github.com/codecrafters-io/bittorrent-starter-go/internal"
)

func main() {
	fmt.Fprintln(os.Stderr, "Logs from your program will appear here!")

	command := os.Args[1]

	switch command {
	case "decode":
		fmt.Println(internal.DecodeBencode(os.Args[2]))
	case "info":
		fmt.Println(internal.ShowTorrentInfo(os.Args[2]))
	case "peers":
		fmt.Println(internal.DetectPeers(os.Args[2]))
	default:
		log.Fatalf("Unknown command: %q", command)
	}
}
