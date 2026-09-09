package main

import (
	"fmt"
	"strings"
)

type Value interface {
	isRespValue()
	encode() []byte
}

type InArray []BulkString

type BulkString []byte
type SimpleString string
type Integer int64
type Array []Value
type SimpleError string

func (BulkString) isRespValue()   {}
func (SimpleString) isRespValue() {}
func (Integer) isRespValue()      {}
func (Array) isRespValue()        {}
func (InArray) isRespValue()      {}
func (SimpleError) isRespValue()  {}

func (arr Array) encode() []byte {
	if arr == nil {
		return []byte("*-1\r\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "*%d%s", len(arr), Separator)

	for _, v := range arr {
		b.Write(v.encode())
	}

	return []byte(b.String())
}

func (bs BulkString) encode() []byte {
	if bs == nil {
		return []byte("$-1\r\n")
	}
	return fmt.Appendf(nil, "$%d%s%s%s", len(bs), Separator, bs, Separator)
}

func (ss SimpleString) encode() []byte {
	return fmt.Appendf(nil, "+%s%s", ss, Separator)
}

func (i Integer) encode() []byte {
	return fmt.Appendf(nil, ":%d%s", i, Separator)
}

func (err SimpleError) encode() []byte {
	return fmt.Appendf(nil, "-ERR %s%s", err, Separator)
}

func (arr InArray) encode() []byte {
	if arr == nil {
		return []byte("*-1\r\n")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "*%d%s", len(arr), Separator)

	for _, v := range arr {
		b.Write(v.encode())
	}

	return []byte(b.String())
}
