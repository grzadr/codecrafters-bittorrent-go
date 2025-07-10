package internal

import "net"

type TorrentRequest struct {
	Conn *net.Conn
	Msg  []byte
	Err  error
}
