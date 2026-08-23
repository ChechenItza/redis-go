package main

import (
	"bufio"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

var SimpleParsingErr = SimpleError(ErrParsing.Error())

type CmdSpec struct {
	arity   func(args InArray) bool
	handler func(b *Backend, args InArray) Value
	isTxCmd bool
}

var commands = map[string]CmdSpec{
	"echo":    {arity: exact(1), handler: (*Backend).echo, isTxCmd: false},
	"ping":    {arity: exact(0), handler: (*Backend).ping, isTxCmd: false},
	"set":     {arity: oneOf(2, 4), handler: (*Backend).set, isTxCmd: false},
	"get":     {arity: exact(1), handler: (*Backend).get, isTxCmd: false},
	"incr":    {arity: exact(1), handler: (*Backend).incr, isTxCmd: false},
	"multi":   {arity: exact(0), handler: (*Backend).multi, isTxCmd: true},
	"exec":    {arity: exact(0), handler: (*Backend).exec, isTxCmd: true},
	"discard": {arity: exact(0), handler: (*Backend).discard, isTxCmd: true},
	"rpush":   {arity: atLeast(2), handler: (*Backend).rpush, isTxCmd: false},
	"lrange":  {arity: exact(3), handler: (*Backend).lrange, isTxCmd: false},
	"lpush":   {arity: atLeast(2), handler: (*Backend).lpush, isTxCmd: false},
	"llen":    {arity: exact(1), handler: (*Backend).llen, isTxCmd: false},
	"lpop":    {arity: oneOf(1, 2), handler: (*Backend).lpop, isTxCmd: false},
	"blpop":   {arity: exact(2), handler: (*Backend).blpop, isTxCmd: false},
}

func exact(n int) func(args InArray) bool {
	return func(args InArray) bool {
		return len(args) == n
	}
}

func oneOf(ns ...int) func(args InArray) bool {
	return func(args InArray) bool {
		return slices.Contains(ns, len(args))
	}
}

func atLeast(n int) func(args InArray) bool {
	return func(args InArray) bool {
		return len(args) >= n
	}
}

func toByteSlices(bs []BulkString) [][]byte {
	out := make([][]byte, len(bs))
	for i, b := range bs {
		out[i] = []byte(b)
	}
	return out
}

func bulkStrings(bs [][]byte) Array {
	res := make(Array, 0, len(bs))
	for _, b := range bs {
		res = append(res, BulkString(b))
	}
	return res
}

type Backend struct {
	storage *InMemory
	txQueue []func() Value
}

func NewBackend(st *InMemory) *Backend {
	return &Backend{
		storage: st,
	}
}

func (b *Backend) Interpret(r *bufio.Reader) ([]byte, error) {
	arr, err := Decode(r)
	if err != nil {
		if errors.Is(err, ErrParsing) {
			return Encode(SimpleParsingErr), nil
		}

		return nil, err
	}

	cmdStr := strings.ToLower(string(arr[0]))
	cmd, ok := commands[cmdStr]
	if !ok {
		return Encode(SimpleParsingErr), nil
	}

	if !cmd.arity(arr[1:]) {
		return Encode(SimpleParsingErr), nil
	}

	if b.txQueue != nil && !cmd.isTxCmd {
		b.txQueue = append(b.txQueue, func() Value {
			return cmd.handler(b, arr)
		})
		return Encode(SimpleString("QUEUED")), nil
	}

	res := cmd.handler(b, arr)
	return Encode(res), nil
}

func (b *Backend) echo(args InArray) Value {
	return args[1]
}

func (b *Backend) ping(_ InArray) Value {
	return SimpleString("PONG")
}

func (b *Backend) set(args InArray) Value {
	k := string(args[1])
	v := []byte(args[2])
	var ttl *time.Duration
	if len(args) > 3 {
		ttlKind := strings.ToLower(string(args[3]))
		n, err := strconv.Atoi(string(args[4]))
		if err != nil {
			return SimpleParsingErr
		}

		switch ttlKind {
		case "ex":
			ttlDur := time.Second * time.Duration(n)
			ttl = &ttlDur
		case "px":
			ttlDur := time.Millisecond * time.Duration(n)
			ttl = &ttlDur
		default:
			return SimpleParsingErr
		}
	}

	b.storage.store(k, v, ttl)
	return SimpleString("OK")
}

func (b *Backend) get(args InArray) Value {
	k := string(args[1])
	v, err := b.storage.get(k)
	if err != nil {
		return SimpleError(err.Error())
	}

	return BulkString(v)
}

func (b *Backend) incr(args InArray) Value {
	k := string(args[1])

	v, err := b.storage.increase(k)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(v)
}

func (b *Backend) multi(_ InArray) Value {
	if b.txQueue != nil {
		return SimpleError("recusive transactions are not allowed")
	}

	b.txQueue = make([]func() Value, 0)
	return SimpleString("OK")
}

func (b *Backend) exec(_ InArray) Value {
	if b.txQueue == nil {
		return SimpleError("EXEC without MULTI")
	}

	res := make(Array, 0)
	for _, f := range b.txQueue {
		res = append(res, f())
	}

	b.txQueue = nil

	return res
}

func (b *Backend) discard(_ InArray) Value {
	if b.txQueue == nil {
		return SimpleError("DISCARD without MULTI")
	}

	b.txQueue = nil
	return SimpleString("OK")
}

func (b *Backend) rpush(args InArray) Value {
	k := string(args[1])
	v := toByteSlices(args[2:])

	n, err := b.storage.rpush(k, v)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(n)
}

func (b *Backend) lrange(args InArray) Value {
	k := string(args[1])
	startBytes := args[2]
	endBytes := args[3]

	start, err := strconv.Atoi(string(startBytes))
	if err != nil {
		return SimpleError(ErrInvalidInt.Error())
	}

	end, err := strconv.Atoi(string(endBytes))
	if err != nil {
		return SimpleError(ErrInvalidInt.Error())
	}

	arr, err := b.storage.lrange(k, start, end)
	if err != nil {
		return SimpleError(err.Error())
	}

	return bulkStrings(arr)
}

func (b *Backend) lpush(args InArray) Value {
	k := string(args[1])
	v := toByteSlices(args[2:])

	n, err := b.storage.lpush(k, v)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(n)
}

func (b *Backend) llen(args InArray) Value {
	k := string(args[1])

	n, err := b.storage.llen(k)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(n)
}

func (b *Backend) lpop(args InArray) Value {
	k := string(args[1])
	if len(args) > 2 {
		iStr := args[2]
		i, err := strconv.Atoi(string(iStr))
		if err != nil {
			return SimpleError(ErrInvalidInt.Error())
		}

		arr, err := b.storage.lpopN(k, i)
		if err != nil {
			return SimpleError(err.Error())
		}

		return bulkStrings(arr)
	}

	n, err := b.storage.lpop(k)
	if err != nil {
		return SimpleError(err.Error())
	}

	return BulkString(n)
}

func (b *Backend) blpop(args InArray) Value {
	k := string(args[1])
	ttlSec, err := strconv.ParseFloat(string(args[2]), 64)
	if err != nil {
		return SimpleError(ErrInvalidInt.Error())
	}

	v, ch, err := b.storage.blpop(k, time.Duration(ttlSec*float64(time.Second)))
	if err != nil {
		return SimpleError(err.Error())
	}

	if ch != nil {
		v = <-ch
	}

	if v == nil {
		return Array(nil)
	}

	return Array([]Value{args[1], BulkString(v)})
}
