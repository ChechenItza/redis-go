package main

import (
	"bufio"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

const SubBufferSize = 128

var SimpleParsingErr = SimpleError(ErrParsing.Error())

type CmdSpec struct {
	arity       func(args InArray) bool
	handler     func(b *ConnState, args InArray) Value
	isTxCmd     bool
	isPubSubCmd bool
}

var commands = map[string]CmdSpec{
	"echo":        {arity: exact(1), handler: (*ConnState).echo},
	"ping":        {arity: exact(0), handler: (*ConnState).ping, isPubSubCmd: true},
	"set":         {arity: oneOf(2, 4), handler: (*ConnState).set},
	"get":         {arity: exact(1), handler: (*ConnState).get},
	"incr":        {arity: exact(1), handler: (*ConnState).incr},
	"multi":       {arity: exact(0), handler: (*ConnState).multi, isTxCmd: true},
	"exec":        {arity: exact(0), handler: (*ConnState).exec, isTxCmd: true},
	"discard":     {arity: exact(0), handler: (*ConnState).discard, isTxCmd: true},
	"rpush":       {arity: atLeast(2), handler: (*ConnState).rpush},
	"lrange":      {arity: exact(3), handler: (*ConnState).lrange},
	"lpush":       {arity: atLeast(2), handler: (*ConnState).lpush},
	"llen":        {arity: exact(1), handler: (*ConnState).llen},
	"lpop":        {arity: oneOf(1, 2), handler: (*ConnState).lpop},
	"blpop":       {arity: exact(2), handler: (*ConnState).blpop},
	"subscribe":   {arity: exact(1), handler: (*ConnState).subscribe, isPubSubCmd: true},
	"publish":     {arity: exact(2), handler: (*ConnState).publish, isPubSubCmd: true},
	"unsubscribe": {arity: exact(1), handler: (*ConnState).unsubscribe, isPubSubCmd: true},
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

type ConnState struct {
	storage *InMemory
	txQueue []func() Value
	pubsub  *PubSub
	ctx     context.Context

	isSubState    bool
	subscriber    *Subscriber
	subscriptions map[string]struct{}
}

func NewConnState(ctx context.Context, st *InMemory, ps *PubSub) *ConnState {
	return &ConnState{
		storage: st,
		pubsub:  ps,
		ctx:     ctx,
		subscriber: &Subscriber{
			ch:   make(chan Message, SubBufferSize),
			dead: make(chan struct{}),
		},
		subscriptions: make(map[string]struct{}),
	}
}

func (b *ConnState) Interpret(r *bufio.Reader) ([]byte, error) {
	arr, err := Decode(r)
	if err != nil {
		if errors.Is(err, ErrParsing) {
			return SimpleParsingErr.encode(), nil
		}

		return nil, err
	}

	cmdStr := strings.ToLower(string(arr[0]))
	cmd, ok := commands[cmdStr]
	if !ok {
		return SimpleParsingErr.encode(), nil
	}

	if !cmd.arity(arr[1:]) {
		return SimpleParsingErr.encode(), nil
	}

	if b.txQueue != nil && !cmd.isTxCmd {
		b.txQueue = append(b.txQueue, func() Value {
			return cmd.handler(b, arr)
		})
		return SimpleString("QUEUED").encode(), nil
	}

	if b.isSubState && !cmd.isPubSubCmd {
		return SimpleError("Can't execute '" + cmdStr + "'").encode(), nil
	}

	res := cmd.handler(b, arr)
	return res.encode(), nil
}

func (b *ConnState) echo(args InArray) Value {
	return args[1]
}

func (b *ConnState) ping(_ InArray) Value {
	if b.isSubState {
		return Array([]Value{BulkString("pong"), BulkString("")})
	}

	return SimpleString("PONG")
}

func (b *ConnState) set(args InArray) Value {
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

func (b *ConnState) get(args InArray) Value {
	k := string(args[1])
	v, err := b.storage.get(k)
	if err != nil {
		return SimpleError(err.Error())
	}

	return BulkString(v)
}

func (b *ConnState) incr(args InArray) Value {
	k := string(args[1])

	v, err := b.storage.increase(k)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(v)
}

func (b *ConnState) multi(_ InArray) Value {
	if b.txQueue != nil {
		return SimpleError("recusive transactions are not allowed")
	}

	b.txQueue = make([]func() Value, 0)
	return SimpleString("OK")
}

func (b *ConnState) exec(_ InArray) Value {
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

func (b *ConnState) discard(_ InArray) Value {
	if b.txQueue == nil {
		return SimpleError("DISCARD without MULTI")
	}

	b.txQueue = nil
	return SimpleString("OK")
}

func (b *ConnState) rpush(args InArray) Value {
	k := string(args[1])
	v := toByteSlices(args[2:])

	n, err := b.storage.rpush(k, v)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(n)
}

func (b *ConnState) lrange(args InArray) Value {
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

func (b *ConnState) lpush(args InArray) Value {
	k := string(args[1])
	v := toByteSlices(args[2:])

	n, err := b.storage.lpush(k, v)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(n)
}

func (b *ConnState) llen(args InArray) Value {
	k := string(args[1])

	n, err := b.storage.llen(k)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(n)
}

func (b *ConnState) lpop(args InArray) Value {
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

func (b *ConnState) blpop(args InArray) Value {
	k := string(args[1])
	ttlSec, err := strconv.ParseFloat(string(args[2]), 64)
	if err != nil {
		return SimpleError(ErrInvalidInt.Error())
	}

	v, err := b.storage.blpop(b.ctx, k, time.Duration(ttlSec*float64(time.Second)))
	if err != nil {
		return SimpleError(err.Error())
	}
	if v == nil {
		return Array(nil)
	}

	return Array([]Value{args[1], BulkString(v)})
}

func (b *ConnState) subscribe(args InArray) Value {
	channel := string(args[1])

	b.isSubState = true

	b.pubsub.subscribe(channel, b.subscriber)
	b.subscriptions[channel] = struct{}{}

	return Array(
		[]Value{BulkString("subscribe"),
			BulkString(channel),
			Integer(len(b.subscriptions))},
	)
}

func (b *ConnState) publish(args InArray) Value {
	channel := string(args[1])
	msg := args[2]

	n, err := b.pubsub.publish(channel, msg)
	if err != nil {
		return SimpleError(err.Error())
	}

	return Integer(n)
}

func (b *ConnState) unsubscribe(args InArray) Value {
	channel := string(args[1])
	if !b.isSubState {
		return SimpleError(ErrNeverSubscribed.Error())
	}

	b.pubsub.unsubscribe(channel, b.subscriber)
	delete(b.subscriptions, channel)

	if len(b.subscriptions) == 0 {
		b.isSubState = false
	}

	return Array(
		[]Value{BulkString("unsubscribe"),
			BulkString(channel),
			Integer(len(b.subscriptions))},
	)
}

func (b *ConnState) Listen() ([]byte, error) {
	select {
	case msg := <-b.subscriber.ch:
		return Array(
			[]Value{
				BulkString("message"),
				BulkString(msg.channel),
				BulkString(msg.v),
			},
		).encode(), nil
	case <-b.subscriber.dead:
		return nil, ErrSubBuffExceeded
	}
}

func (b *ConnState) Teardown() {
	b.subscriber.kill()
	b.pubsub.unsubscribeFromAll(b.subscriber)
}
