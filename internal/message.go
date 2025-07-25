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
				log.Println("EOF:", hex.Dump(sizeBuf))

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

			msg.content = make([]byte, msg.Size)

			_, err = io.ReadFull(reader, msg.content)
			if err != nil {
				msg.Err = fmt.Errorf("failed to read %d bytes: %w", length, err)
			}

			// msg.content = contentBuf[:n]

			if !yield(msg) {
				return
			}
		}
	}
}

type RequestMessage struct {
	index int
	begin int
	block int
}

func (m RequestMessage) encode() (msg []byte) {
	contentLength := int32Size*requestFieldsNum + 1
	msg = make([]byte, msgLengthBytes+contentLength)
	intToBytes(contentLength, msg)
	msg[msgLengthBytes] = byte(Request)

	intToBytes(m.index, msg[msgLengthBytes+1:])
	intToBytes(m.begin, msg[msgLengthBytes+int32Size+1:])
	intToBytes(m.block, msg[msgLengthBytes+int32Size*2+1:])

	return
}

type PieceMessage struct {
	index int
	begin int
	block []byte
}

func NewPiecePayload(data []byte) (piece PieceMessage) {
	log.Println("NewPiecePayload\n", hex.Dump(data[:16]))
	// msg, _ := NewMessage(data)

	// if msg.msgType != Piece {
	// 	panic(fmt.Sprintf("expected %q, got %q", Piece, msg.msgType))
	// }

	piece.index = bytesToInt(data[:int32Size])
	piece.begin = bytesToInt(data[int32Size : int32Size*2])
	piece.block = data[int32Size*2:]

	return
}
