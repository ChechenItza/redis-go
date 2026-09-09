package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
)

const Separator = "\r\n"

const ArrayPrefix = '*'
const BulkStringPrefix = '$'

const ArrayMaxElems = 1_000_000
const ArrayMinElems = 1
const BulkStringMaxBytes = 512_000_000
const BulkStringMinBytes = 0

func Decode(r *bufio.Reader) (InArray, error) {
	arr, err := parseArray(r)
	if err != nil {
		return nil, err
	}

	return arr, nil
}

func parsePrefixedCount(r *bufio.Reader, prefix byte) (int, error) {
	prefixedCount, err := r.ReadBytes('\n')
	if err != nil {
		return -1, err
	}

	if prefixedCount[0] != prefix {
		return -1, fmt.Errorf("got %c, expected %c: %w", prefixedCount[0], prefix, ErrBadPrefix)
	}
	if len(prefixedCount) < 4 {
		return -1, ErrNegativeCount
	}

	// trim \r\n
	prefixedCount = prefixedCount[1 : len(prefixedCount)-2]
	n, err := strconv.Atoi(string(prefixedCount))
	if err != nil {
		return -1, fmt.Errorf("got %q: %w", prefixedCount, ErrBadLength)
	}

	return n, nil
}

func parseArray(r *bufio.Reader) (InArray, error) {
	n, err := parsePrefixedCount(r, ArrayPrefix)
	if err != nil {
		return nil, err
	}
	if n < ArrayMinElems || n > ArrayMaxElems {
		return nil, ErrBadLength
	}

	res := make([]BulkString, 0, n)
	for i := 0; i < n; i++ {
		bs, err := parseBulkString(r)
		if err != nil {
			return nil, err
		}
		res = append(res, bs)
	}

	return res, nil
}

func parseBulkString(r *bufio.Reader) (BulkString, error) {
	n, err := parsePrefixedCount(r, BulkStringPrefix)
	if err != nil {
		return nil, err
	}
	if n < BulkStringMinBytes || n > BulkStringMaxBytes {
		return nil, ErrBadLength
	}

	bs := make(BulkString, n)
	_, err = io.ReadFull(r, bs)
	if err != nil {
		return nil, err
	}

	_, err = r.Discard(2)
	if err != nil {
		return nil, err
	}

	return bs, nil
}
