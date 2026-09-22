package main

import (
	"bufio"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chechenitza/redis-go/app/resp"
)

const SubBufferSize = 128

var SimpleParsingErr = resp.SimpleError(ErrParsing.Error())

type CmdSpec struct {
	arity       func(args resp.InArray) bool
	handler     func(b *ConnState, args resp.InArray) resp.Value
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
	"zadd":        {arity: exact(3), handler: (*ConnState).zadd},
	"zrank":       {arity: exact(2), handler: (*ConnState).zrank},
	"zrange":      {arity: exact(3), handler: (*ConnState).zrange},
	"zcard":       {arity: exact(1), handler: (*ConnState).zcard},
	"zscore":      {arity: exact(2), handler: (*ConnState).zscore},
	"zrem":        {arity: exact(2), handler: (*ConnState).zrem},
}

func exact(n int) func(args resp.InArray) bool {
	return func(args resp.InArray) bool {
		return len(args) == n
	}
}

func oneOf(ns ...int) func(args resp.InArray) bool {
	return func(args resp.InArray) bool {
		return slices.Contains(ns, len(args))
	}
}

func atLeast(n int) func(args resp.InArray) bool {
	return func(args resp.InArray) bool {
		return len(args) >= n
	}
}

func toByteSlices(bs []resp.BulkString) [][]byte {
	out := make([][]byte, len(bs))
	for i, b := range bs {
		out[i] = []byte(b)
	}
	return out
}

func bulkStrings(bs [][]byte) resp.Array {
	res := make(resp.Array, 0, len(bs))
	for _, b := range bs {
		res = append(res, resp.BulkString(b))
	}
	return res
}

func stringsToArray(ss []string) resp.Array {
	res := make(resp.Array, 0, len(ss))
	for _, s := range ss {
		res = append(res, resp.BulkString(s))
	}
	return res
}

func emptyArray() resp.Array {
	return resp.Array(make([]resp.Value, 0))
}

type ConnState struct {
	storage *InMemory
	txQueue []func() resp.Value
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
	arr, err := resp.ParseArray(r)
	if err != nil {
		if errors.Is(err, ErrParsing) {
			return SimpleParsingErr.Encode(), nil
		}

		return nil, err
	}

	cmdStr := strings.ToLower(string(arr[0]))
	cmd, ok := commands[cmdStr]
	if !ok {
		return SimpleParsingErr.Encode(), nil
	}

	if !cmd.arity(arr[1:]) {
		return SimpleParsingErr.Encode(), nil
	}

	if b.txQueue != nil && !cmd.isTxCmd {
		b.txQueue = append(b.txQueue, func() resp.Value {
			return cmd.handler(b, arr)
		})
		return resp.SimpleString("QUEUED").Encode(), nil
	}

	if b.isSubState && !cmd.isPubSubCmd {
		return resp.SimpleError("Can't execute '" + cmdStr + "'").Encode(), nil
	}

	res := cmd.handler(b, arr)
	return res.Encode(), nil
}

func (b *ConnState) echo(args resp.InArray) resp.Value {
	return args[1]
}

func (b *ConnState) ping(_ resp.InArray) resp.Value {
	if b.isSubState {
		return resp.Array([]resp.Value{resp.BulkString("pong"), resp.BulkString("")})
	}

	return resp.SimpleString("PONG")
}

func (b *ConnState) set(args resp.InArray) resp.Value {
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
	return resp.SimpleString("OK")
}

func (b *ConnState) get(args resp.InArray) resp.Value {
	k := string(args[1])
	v, err := b.storage.get(k)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.BulkString(v)
}

func (b *ConnState) incr(args resp.InArray) resp.Value {
	k := string(args[1])

	v, err := b.storage.increase(k)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.Integer(v)
}

func (b *ConnState) multi(_ resp.InArray) resp.Value {
	if b.txQueue != nil {
		return resp.SimpleError("recusive transactions are not allowed")
	}

	b.txQueue = make([]func() resp.Value, 0)
	return resp.SimpleString("OK")
}

func (b *ConnState) exec(_ resp.InArray) resp.Value {
	if b.txQueue == nil {
		return resp.SimpleError("EXEC without MULTI")
	}

	res := make(resp.Array, 0)
	for _, f := range b.txQueue {
		res = append(res, f())
	}

	b.txQueue = nil

	return res
}

func (b *ConnState) discard(_ resp.InArray) resp.Value {
	if b.txQueue == nil {
		return resp.SimpleError("DISCARD without MULTI")
	}

	b.txQueue = nil
	return resp.SimpleString("OK")
}

func (b *ConnState) rpush(args resp.InArray) resp.Value {
	k := string(args[1])
	v := toByteSlices(args[2:])

	n, err := b.storage.rpush(k, v)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.Integer(n)
}

func (b *ConnState) lrange(args resp.InArray) resp.Value {
	k := string(args[1])
	startBytes := args[2]
	endBytes := args[3]

	start, err := strconv.Atoi(string(startBytes))
	if err != nil {
		return resp.SimpleError(ErrInvalidInt.Error())
	}

	end, err := strconv.Atoi(string(endBytes))
	if err != nil {
		return resp.SimpleError(ErrInvalidInt.Error())
	}

	arr, err := b.storage.lrange(k, start, end)
	if err != nil {
		return emptyArray()
	}

	return bulkStrings(arr)
}

func (b *ConnState) lpush(args resp.InArray) resp.Value {
	k := string(args[1])
	v := toByteSlices(args[2:])

	n, err := b.storage.lpush(k, v)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.Integer(n)
}

func (b *ConnState) llen(args resp.InArray) resp.Value {
	k := string(args[1])

	n, err := b.storage.llen(k)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.Integer(n)
}

func (b *ConnState) lpop(args resp.InArray) resp.Value {
	k := string(args[1])
	if len(args) > 2 {
		iStr := args[2]
		i, err := strconv.Atoi(string(iStr))
		if err != nil {
			return resp.SimpleError(ErrInvalidInt.Error())
		}

		arr, err := b.storage.lpopN(k, i)
		if err != nil {
			return resp.SimpleError(err.Error())
		}

		return bulkStrings(arr)
	}

	n, err := b.storage.lpop(k)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.BulkString(n)
}

func (b *ConnState) blpop(args resp.InArray) resp.Value {
	k := string(args[1])
	ttlSec, err := strconv.ParseFloat(string(args[2]), 64)
	if err != nil {
		return resp.SimpleError(ErrInvalidInt.Error())
	}

	v, err := b.storage.blpop(b.ctx, k, time.Duration(ttlSec*float64(time.Second)))
	if err != nil {
		return resp.SimpleError(err.Error())
	}
	if v == nil {
		return resp.Array(nil)
	}

	return resp.Array([]resp.Value{args[1], resp.BulkString(v)})
}

func (b *ConnState) subscribe(args resp.InArray) resp.Value {
	channel := string(args[1])

	b.isSubState = true

	b.pubsub.subscribe(channel, b.subscriber)
	b.subscriptions[channel] = struct{}{}

	return resp.Array(
		[]resp.Value{resp.BulkString("subscribe"),
			resp.BulkString(channel),
			resp.Integer(len(b.subscriptions))},
	)
}

func (b *ConnState) publish(args resp.InArray) resp.Value {
	channel := string(args[1])
	msg := args[2]

	n, err := b.pubsub.publish(channel, msg)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.Integer(n)
}

func (b *ConnState) unsubscribe(args resp.InArray) resp.Value {
	channel := string(args[1])
	if !b.isSubState {
		return resp.SimpleError(ErrNeverSubscribed.Error())
	}

	b.pubsub.unsubscribe(channel, b.subscriber)
	delete(b.subscriptions, channel)

	if len(b.subscriptions) == 0 {
		b.isSubState = false
	}

	return resp.Array(
		[]resp.Value{resp.BulkString("unsubscribe"),
			resp.BulkString(channel),
			resp.Integer(len(b.subscriptions))},
	)
}

func (b *ConnState) Listen() ([]byte, error) {
	select {
	case msg := <-b.subscriber.ch:
		return resp.Array(
			[]resp.Value{
				resp.BulkString("message"),
				resp.BulkString(msg.channel),
				resp.BulkString(msg.v),
			},
		).Encode(), nil
	case <-b.subscriber.dead:
		return nil, ErrSubBuffExceeded
	}
}

func (b *ConnState) zadd(args resp.InArray) resp.Value {
	k := string(args[1])
	scoreBytes := args[2]
	nameBytes := args[3]

	score, err := strconv.ParseFloat(string(scoreBytes), 64)
	if err != nil {
		return resp.SimpleError(ErrInvalidInt.Error())
	}

	name := string(nameBytes)

	n, err := b.storage.zadd(k, score, name)
	if err != nil {
		return resp.SimpleError(err.Error())
	}

	return resp.Integer(n)
}

func (b *ConnState) zrank(args resp.InArray) resp.Value {
	key := string(args[1])
	name := string(args[2])

	n, err := b.storage.zrank(key, name)
	if err != nil {
		return resp.BulkString(nil)
	}

	return resp.Integer(n)
}

func (b *ConnState) zrange(args resp.InArray) resp.Value {
	key := string(args[1])
	iBytes := args[2]
	jBytes := args[3]

	i, err := strconv.Atoi(string(iBytes))
	if err != nil {
		return resp.SimpleError(ErrInvalidInt.Error())
	}
	j, err := strconv.Atoi(string(jBytes))
	if err != nil {
		return resp.SimpleError(ErrInvalidInt.Error())
	}

	ss, err := b.storage.zrange(key, i, j)
	if err != nil {
		return resp.Array([]resp.Value{})
	}

	return stringsToArray(ss)
}

func (b *ConnState) zcard(args resp.InArray) resp.Value {
	key := string(args[1])

	n, err := b.storage.zcard(key)
	if err != nil {
		return resp.Integer(0)
	}

	return resp.Integer(n)
}

func (b *ConnState) zscore(args resp.InArray) resp.Value {
	key := string(args[1])
	member := string(args[2])

	score, err := b.storage.zscore(key, member)
	if err != nil {
		return resp.BulkString(nil)
	}

	return resp.BulkString(strconv.FormatFloat(score, 'f', -1, 64))
}

func (b *ConnState) zrem(args resp.InArray) resp.Value {
	key := string(args[1])
	member := string(args[2])

	err := b.storage.zrem(key, member)
	if err != nil {
		return resp.Integer(0)
	}

	return resp.Integer(1)
}

func (b *ConnState) Teardown() {
	b.subscriber.kill()
	b.pubsub.unsubscribeFromAll(b.subscriber)
}
