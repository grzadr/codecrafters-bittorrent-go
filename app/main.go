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
		fmt.Println(internal.CmdDecode(os.Args[2]))
	case "info":
		fmt.Println(internal.CmdInfo(os.Args[2]))
	case "peers":
		fmt.Println(internal.CmdPeers(os.Args[2]))
	case "handshake":
		fmt.Println(internal.CmdHandshake(os.Args[2], os.Args[3]))
	case "download_piece":
		internal.CmdDownloadPiece(os.Args[3], os.Args[4], os.Args[5])
	case "download":
		internal.CmdDownload(os.Args[3], os.Args[4])
	default:
		log.Fatalf("Unknown command: %q", command)
	}
}
