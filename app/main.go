package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

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
		index, _ := strconv.Atoi(os.Args[5])
		internal.CmdDownloadPiece(os.Args[3], os.Args[4], index)
	case "download":
		internal.CmdDownload(os.Args[3], os.Args[4])
	case "magnet_parse":
		fmt.Println(internal.CmdMagnetParse(os.Args[2]))
	default:
		log.Fatalf("Unknown command: %q", command)
	}
}
