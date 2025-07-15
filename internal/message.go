package internal

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"iter"
	"log"
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

func NewHandshakeMessage(reader *bufio.Reader) (msg Message) {
	msg = Message{
		Type:    Handshake,
		content: make([]byte, handshakeMsgLength),
		Size:    handshakeMsgLength,
	}
	copy(msg.content, []byte(handshakePrefix))

	if _, msg.Err = io.ReadFull(reader, msg.content[len(handshakePrefix):]); msg.Err != nil {
		msg.Err = fmt.Errorf("error reading handshake response: %w", msg.Err)

		return msg
	}

	log.Println(hex.Dump(msg.content))

	return
}

func NewMessage(reader *bufio.Reader) iter.Seq[Message] {
	return func(yield func(Message) bool) {
		sizeBuf := make([]byte, int32Size)

		for {
			msg := Message{}

			if _, msg.Err = io.ReadFull(reader, sizeBuf); msg.Err == io.EOF {
				log.Println(hex.Dump(sizeBuf))
				log.Println("EOF")

				return
			} else if msg.Err != nil {
				msg.Err = fmt.Errorf("error reading length: %w", msg.Err)

				yield(msg)

				return
			}

			log.Println(hex.Dump(sizeBuf))

			if bytes.Equal(sizeBuf, []byte(handshakePrefix)) {
				if !yield(NewHandshakeMessage(reader)) {
					return
				}

				continue
			}

			length := bytesToInt(sizeBuf)

			if length == 0 {
				continue
			}

			msg.Size = length - 1

			msgByte, err := reader.ReadByte()
			if err != nil {
				msg.Err = fmt.Errorf("error reading msg type: %w", err)

				yield(msg)

				return
			}

			msg.Type = MessageType(msgByte)

			contentBuf := make([]byte, msg.Size)

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
