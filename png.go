/*
  Portable-stripped down implementation of the PNG file format for simple verification
*/
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

var (
	ErrorMissingBytes = errors.New("missing bytes")
	ErrorCRCMismatch  = errors.New("crc mismatch")

	ErrorNotPNG              = errors.New("not a png file")
	ErrorInvalidHeaderLength = errors.New("invalid header length")
	ErrorDOSToUnixConversion = errors.New("dos to unix conversion")
	ErrorUnixToDOSConversion = errors.New("unix to dos conversion")

	ErrorNoMissingBytes = errors.New("no missing bytes")
)

var PNGHeader = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

//Chunk represents a chunk within the PNG file
type Chunk struct {
	Length uint32
	Type   string
	Data   []byte
	CRC    uint32
}

func (c *Chunk) Write(w io.Writer) {
	binary.Write(w, binary.BigEndian, c.Length)
	binary.Write(w, binary.BigEndian, []byte(c.Type))
	binary.Write(w, binary.BigEndian, c.Data)
	binary.Write(w, binary.BigEndian, c.CRC)
}

//Verify attempts to verify the chunk with the CRC & Length of the file
func (c *Chunk) Verify() (uint32, error) {

	if int(c.Length) != len(c.Data) {
		// woop. missing bytes
		return 0, ErrorMissingBytes
	}

	dataCrc := crc32.ChecksumIEEE(bytes.Join([][]byte{[]byte(c.Type), c.Data}, []byte{}))
	if dataCrc != c.CRC {
		return dataCrc, ErrorCRCMismatch
	}

	return 0, nil
}

type Header struct {
	HeaderBytes []byte
}

func (h *Header) Verify() error {
	if h.HeaderBytes[0] != 0x89 && string(h.HeaderBytes[1:4]) != "PNG" {
		return ErrorNotPNG
	}

	if len(h.HeaderBytes) < 8 {
		if h.HeaderBytes[5] != 0x0D {
			return ErrorDOSToUnixConversion
		}
		return ErrorInvalidHeaderLength
	}

	if h.HeaderBytes[7] != 0x0A {
		return ErrorUnixToDOSConversion
	}
	return nil
}

//PNG represents the PNG file structure
type PNG struct {
	FileHeader *Header
	Chunks     map[string][]*Chunk
}

func Read(reader io.Reader) (*PNG, error) {
	buf := bufio.NewReader(reader)

	magicHeader := make([]byte, 8)
	buf.Read(magicHeader)

	var chunks = map[string][]*Chunk{}
	localBuffer := make([]byte, 4)

	for {
		var length uint32
		var chunkType string
		var data []byte
		var crc uint32

		binary.Read(buf, binary.BigEndian, &length)

		buf.Read(localBuffer)
		chunkType = string(localBuffer)

		data = make([]byte, length)

		io.ReadFull(buf, data)

		binary.Read(buf, binary.BigEndian, &crc)

		checkSumMe := []byte(chunkType)

		checkSumMe = append(checkSumMe, data...)

		ourCrc := crc32.ChecksumIEEE(checkSumMe)

		if ourCrc != crc {
			return nil, ErrorCRCMismatch
		}

		chunk := &Chunk{
			Length: length,
			Type:   chunkType,
			Data:   data,
			CRC:    crc,
		}

		if _, ok := chunks[chunkType]; !ok {
			chunks[chunkType] = make([]*Chunk, 0)
		}

		v := chunks[chunkType]
		v = append(v, chunk)
		chunks[chunkType] = v

		if chunkType == "IEND" {
			break
		}
	}

	return &PNG{
		FileHeader: &Header{magicHeader},
		Chunks:     chunks,
	}, nil
}
