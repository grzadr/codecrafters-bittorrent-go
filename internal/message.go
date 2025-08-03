package internal

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"iter"
	"slices"
)

const (
	handshakeMsgLength = 68
	handshakePrefix    = "\x13Bit"
	int32Size          = 4
	msgLengthBytes     = 4
	requestFieldsNum   = 3
	// protocoLength      = 19.
	protocolName = "\x13BitTorrent protocol"
	// magnetExtensionPos  = 5.
	magnetExtensionFlag = 0x10
	// 	handshakePrefix       = "\x13Bit"
	// 	handshakePrefixLength = len(handshakePrefix)
)

var (
	protocolReservedBytes = [8]byte{}
	protocolNameBytes     = []byte(protocolName)
	magnetExtensionPos    = len(protocolNameBytes) + 5
)

func NewHandshakeRequest(hash Hash) []byte {
	message := make([]byte, handshakeMsgLength)
	// message[0] = byte(len(prto))
	offset := 0
	offset += copy(message[offset:], protocolNameBytes)
	offset += copy(message[offset:], protocolReservedBytes[:])
	offset += copy(message[offset:], hash[:])
	copy(message[offset:], []byte(defaultClientId))

	return message
}

func NewHandshakeRequestExt(hash Hash) []byte {
	handshake := NewHandshakeRequest(hash)

	handshake[magnetExtensionPos] = magnetExtensionFlag

	return handshake
}

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
	Extension = 20
)

type ExtensionType int

const (
	ExtensionHandshake ExtensionType = iota
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
	if m == Extension {
		return "Extension"
	}

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

	return
}

func NewMessage(reader *bufio.Reader) iter.Seq[Message] {
	return func(yield func(Message) bool) {
		sizeBuf := make([]byte, int32Size)

		for {
			msg := Message{}

			if _, msg.Err = io.ReadFull(reader, sizeBuf); msg.Err == io.EOF {
				return
			} else if msg.Err != nil {
				msg.Err = fmt.Errorf("error reading length: %w", msg.Err)
				yield(msg)

				return
			}

			if bytes.Equal(sizeBuf, []byte(handshakePrefix)) {
				if !yield(NewHandshakeMessage(reader)) {
					return
				}

				continue
			}

			length := bytesToInt(sizeBuf)

			msg.Size = length - 1

			msgByte, err := reader.ReadByte()
			if err != nil {
				msg.Err = fmt.Errorf("error reading msg type: %w", err)

				yield(msg)

				return
			}

			msg.Type = MessageType(msgByte)

			msg.content = make([]byte, msg.Size)

			if n, err := io.ReadFull(reader, msg.content); err != nil {
				msg.Err = fmt.Errorf("failed to read %d bytes: %w", length, err)
				yield(msg)

				return
			} else if n != msg.Size {
				msg.Err = fmt.Errorf("failed to read %d bytes: read only %d bytes", length, n)
				yield(msg)

				return
			}

			// msg.content = contentBuf[:n]

			if !yield(msg) {
				return
			}
		}
	}
}

func NewInterestedMsg() Message {
	return Message{
		Type:    Interested,
		content: []byte{},
	}
}

func newExtensionHandshake() Message {
	payload := slices.Concat(
		[]byte{0},
		BencodedMap{
			"m": BencodedMap{
				"ut_metadata": BencodedInteger(1),
			},
		}.Encode())

	return Message{
		Type:    Extension,
		content: payload,
	}
}

func (m Message) encode() (msg []byte) {
	contentLength := len(m.content) + 1
	msg = make([]byte, msgLengthBytes+contentLength)
	intToBytes(contentLength, msg)
	msg[msgLengthBytes] = byte(m.Type)
	copy(msg[msgLengthBytes+1:], m.content[:])

	return
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

func (p RequestMessage) key() BlockKey {
	return BlockKey{index: p.index, begin: p.begin}
}

type PieceMessage struct {
	index int
	begin int
	block []byte
}

func NewPiecePayload(data []byte) (piece PieceMessage) {
	piece.index = bytesToInt(data[:int32Size])
	piece.begin = bytesToInt(data[int32Size : int32Size*2])
	piece.block = data[int32Size*2:]

	return
}

func (p PieceMessage) key() BlockKey {
	return BlockKey{index: p.index, begin: p.begin}
}
