package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
)

type ByteIterator struct {
	data   []byte
	offset int
}

func NewByteIterator(input string) *ByteIterator {
	return &ByteIterator{
		data: []byte(input),
	}
}

func NewByteIteratorBytes(input []byte) *ByteIterator {
	return &ByteIterator{
		data: input,
	}
}

type BencodeType byte

const (
	StringType  BencodeType = 0
	IntegerType BencodeType = 'i'
	ListType    BencodeType = 'l'
	MapType     BencodeType = 'd'
	EndChar     BencodeType = 'e'
	LengthSep   BencodeType = ':'
)

func NewBencodeType(iter *ByteIterator) (t BencodeType, ok bool) {
	if iter.done() {
		return
	}

	ok = true

	switch t = BencodeType(iter.peek()); t {
	case IntegerType, ListType, MapType, EndChar:
		iter.discard(1)

		return
	case LengthSep:
		panic(fmt.Sprintf("unexpected byte %s %+v", string(iter.peek()), t))
	default:
		return StringType, ok
	}
}

func (i ByteIterator) len() int {
	return len(i.data)
}

func (i ByteIterator) left() int {
	return i.len() - i.offset
}

func (i ByteIterator) done() bool {
	return i.left() == 0
}

func (i ByteIterator) peek() byte {
	return i.data[i.offset]
}

func (i *ByteIterator) discard(n int) {
	i.offset += min(i.left(), n)
}

func (i *ByteIterator) readBytes(n int) (b []byte, num int) {
	if i.done() {
		return
	}

	num = min(i.left(), n)

	b = i.data[i.offset : i.offset+num]
	i.offset += num

	return
}

func (i ByteIterator) rest() []byte {
	return i.data[i.offset:]
}

func (i *ByteIterator) readUpTo(delim byte) (b []byte, num int) {
	n := bytes.Index(i.rest(), []byte{delim})
	if n == -1 {
		return
	}

	b, num = i.readBytes(n)

	i.discard(1)

	return
}

type Bencoded interface {
	String() string
	isBencoded()
	Encode() []byte
}

type BencodedString []byte

func NewBencodedString(iter *ByteIterator) BencodedString {
	lenStr, _ := iter.readUpTo(byte(LengthSep))

	length, _ := strconv.Atoi(string(lenStr))
	data, n := iter.readBytes(length)

	if n != length {
		panic(fmt.Sprintf("read %d bytes instead of %d", n, length))
	}

	return BencodedString(data)
}

func (b BencodedString) String() string {
	return string(b)
}

func (b BencodedString) Encode() (encoded []byte) {
	encoded = make([]byte, 0)
	encoded = fmt.Appendf(encoded, "%d:", len(b))
	encoded = append(encoded, []byte(b)...)

	return
}

func (b BencodedString) MarshalText() (text []byte, err error) {
	return []byte(string([]byte(b))), nil
}

func (b BencodedString) isBencoded() {}

type BencodedInteger int

func NewBencodedInteger(iter *ByteIterator) BencodedInteger {
	lenStr, _ := iter.readUpTo(byte(EndChar))
	num, _ := strconv.Atoi(string(lenStr))

	return BencodedInteger(num)
}

func (b BencodedInteger) String() string {
	return string(strconv.Itoa(int(b)))
}

func (b BencodedInteger) Encode() (encoded []byte) {
	// panic("encode")
	encoded = []byte{byte(IntegerType)}
	encoded = fmt.Appendf(encoded, "%d", b)
	encoded = append(encoded, byte(EndChar))

	return
}

func (b BencodedInteger) isBencoded() {}

type BencodedList []Bencoded

func NewBencodedList(iter *ByteIterator) (l BencodedList) {
	l = make(BencodedList, 0)

	for {
		b := NewBencoded(iter)

		if b == nil {
			break
		}

		l = append(l, b)
	}

	return
}

func (b BencodedList) String() string {
	return fmt.Sprintf("%+v", []Bencoded(b))
}

func (b BencodedList) Encode() (encoded []byte) {
	encoded = []byte{byte(ListType)}

	for _, e := range b {
		encoded = append(encoded, e.Encode()...)
	}

	encoded = append(encoded, byte(EndChar))

	return
}

func (b BencodedList) isBencoded() {}

type BencodedMap map[string]Bencoded

func NewBencodedMap(iter *ByteIterator) (m BencodedMap) {
	m = make(BencodedMap)

	for {
		key := NewBencoded(iter)
		value := NewBencoded(iter)

		if key == nil || value == nil {
			break
		}

		m[key.String()] = value
	}

	return
}

func (b BencodedMap) String() string {
	return fmt.Sprintf("%+v", map[string]Bencoded(b))
}

func (b BencodedMap) Encode() (encoded []byte) {
	encoded = []byte{byte(MapType)}

	keys := slices.Sorted(maps.Keys(b))

	for _, key := range keys {
		value := b[key]
		encoded = append(encoded, BencodedString(key).Encode()...)
		encoded = append(encoded, value.Encode()...)
	}

	encoded = append(encoded, byte(EndChar))

	return
}

func (b BencodedMap) isBencoded() {}

func NewBencoded(iter *ByteIterator) Bencoded {
	t, ok := NewBencodeType(iter)
	if !ok {
		return nil
	}

	switch t {
	case StringType:
		return NewBencodedString(iter)
	case IntegerType:
		return NewBencodedInteger(iter)
	case ListType:
		return NewBencodedList(iter)
	case MapType:
		return NewBencodedMap(iter)
	case EndChar:
		return nil
	default:
		panic(fmt.Sprintf("unsupported typ %T %+v", t, iter))
	}
}

func CmdDecode(bencodedString string) string {
	iter := NewByteIterator(bencodedString)

	b := NewBencoded(iter)

	jsonOutput, _ := json.Marshal(b)

	return string(jsonOutput)
}
