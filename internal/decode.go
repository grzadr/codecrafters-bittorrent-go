package internal

import (
	"bytes"
	"fmt"
	"log"
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

	log.Println("up to", n)

	b, num = i.readBytes(n)

	i.discard(1)

	return
}

type Bencoded interface {
	String() string
	isBencoded()
}

type BencodedString string

func NewBencodedString(iter *ByteIterator) BencodedString {
	lenStr, to := iter.readUpTo(byte(LengthSep))
	log.Println(string(lenStr), to)

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

func (b BencodedInteger) isBencoded() {}

type BencodedList []Bencoded

func NewBencodedList(iter *ByteIterator) BencodedList {
	for {
	}

	lenStr, _ := iter.readUpTo(byte(EndChar))
	num, _ := strconv.Atoi(string(lenStr))

	return BencodedInteger(num)
}

func (b BencodedList) String() string {
	return string(strconv.Itoa(int(b)))
}

func (b BencodedList) isBencoded() {}

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
		panic("list not implemented")
	case MapType:
		panic("map not implemented")
	case EndChar:
		return nil
	default:
		panic(fmt.Sprintf("unsupported typ %T %+v", t, iter))
	}
}

func DecodeBencode(bencodedString string) (b Bencoded, err error) {
	iter := NewByteIterator(bencodedString)

	b = NewBencoded(iter)

	return
}
