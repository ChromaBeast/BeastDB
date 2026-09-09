package net

import (
	"bufio"
	"errors"
	"io"
	"strconv"
)

// RespType represents the prefix byte identifying a RESP data type.
type RespType byte

const (
	TypeSimpleString RespType = '+'
	TypeError        RespType = '-'
	TypeInteger      RespType = ':'
	TypeBulkString   RespType = '$'
	TypeArray        RespType = '*'
)

var (
	ErrInvalidRespPrefix = errors.New("resp: invalid type prefix")
	ErrMalformedLine     = errors.New("resp: malformed line terminator")
)

// RespValue encapsulates any valid RESP value.
type RespValue struct {
	Type  RespType
	Str   []byte
	Num   int64
	Array []*RespValue
	Null  bool
}

// ParseResp reads and decodes the next RESP message from the buffered reader.
func ParseResp(r *bufio.Reader) (*RespValue, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	switch RespType(prefix) {
	case TypeSimpleString, TypeError:
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return &RespValue{Type: RespType(prefix), Str: line}, nil

	case TypeInteger:
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		num, err := strconv.ParseInt(string(line), 10, 64)
		if err != nil {
			return nil, err
		}
		return &RespValue{Type: TypeInteger, Num: num}, nil

	case TypeBulkString:
		return parseBulkString(r)

	case TypeArray:
		return parseArray(r)

	default:
		return nil, ErrInvalidRespPrefix
	}
}

func parseBulkString(r *bufio.Reader) (*RespValue, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}

	length, err := strconv.Atoi(string(line))
	if err != nil {
		return nil, err
	}

	// -1 represents Null Bulk String (key not found)
	if length == -1 {
		return &RespValue{Type: TypeBulkString, Null: true}, nil
	}

	buf := make([]byte, length+2) // +2 for trailing \r\n
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	if buf[length] != '\r' || buf[length+1] != '\n' {
		return nil, ErrMalformedLine
	}

	return &RespValue{
		Type: TypeBulkString,
		Str:  buf[:length],
	}, nil
}

func parseArray(r *bufio.Reader) (*RespValue, error) {
	line, err := readLine(r)
	if err != nil {
		return nil, err
	}

	count, err := strconv.Atoi(string(line))
	if err != nil {
		return nil, err
	}

	if count == -1 {
		return &RespValue{Type: TypeArray, Null: true}, nil
	}

	items := make([]*RespValue, count)
	for i := 0; i < count; i++ {
		item, err := ParseResp(r)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}

	return &RespValue{
		Type:  TypeArray,
		Array: items,
	}, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadSlice('\n')
	if err != nil {
		return nil, err
	}
	n := len(line)
	if n < 2 || line[n-2] != '\r' {
		return nil, ErrMalformedLine
	}
	return line[:n-2], nil // Strip trailing \r\n without copying
}
