package internal

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"iter"
)

func intToBytes(n int, buf []byte) {
	binary.BigEndian.PutUint32(buf, uint32(n))
}

func bytesToInt(data []byte) int {
	return int(binary.BigEndian.Uint32(data))
}

type MessageType int

const (
	Choke MessageType = iota
	Unchoke
	Interested
	NotInterested
	Have
	Bitfield
	Request
	Piece
	Cancel
	Handshake
)

var messageTypeNames = [...]string{
	"Choke",
	"Unchoke",
	"Interested",
	"NotInterested",
	"Have",
	"Bitfield",
	"Request",
	"Piece",
	"Cancel",
	"Handshake",
}

func (m MessageType) String() string {
	if m < 0 || int(m) >= len(messageTypeNames) {
		panic(fmt.Sprintf("unknown MessageType(%d)", int(m)))
	}

	return messageTypeNames[m]
}

type Message struct {
	Size    int
	Type    MessageType
	content []byte
	Err     error
}

// type NextMessage struct {
// 	Msg Message
// 	Err error
// }

func NewHandshakeMessage(reader *bufio.Reader) Message {
	buff := make([]byte, 0, handshakeMsgLength-handshakePrefixLength)
}

func NewMessage(reader *bufio.Reader) iter.Seq[Message] {
	return func(yield func(Message) bool) {
		sizeBuf := make([]byte, 0, int32Size)
		contentBuf := make([]byte, 0, defaultByteBuffer)

		for {
			msg := Message{}

			if _, msg.Err = io.ReadFull(reader, sizeBuf); msg.Err == io.EOF {
				return
			} else if msg.Err != nil {
				msg.Err = fmt.Errorf("error reading length: %w", msg.Err)

				yield(msg)

				return
			}

			if bytes.Equal(sizeBuf, handshakePrefix) &&
				!yield(NewHandshakeMessage(reader)) {
				return
			}

			length := bytesToInt(sizeBuf)

			if length == 0 {
				continue
			}

			msgByte, err := reader.ReadByte()
			if err != nil {
				msg.Err = fmt.Errorf("error reading msg type: %w", err)

				yield(msg)

				return
			}

			msg.Type = MessageType(msgByte)

			n, err := reader.Read(contentBuf)
			if err != nil {
				msg.Err = fmt.Errorf("failed to read %d bytes: %w", length, err)
			}

			msg.content = contentBuf[:n]

			if !yield(msg) {
				return
			}
		}
	}
}
