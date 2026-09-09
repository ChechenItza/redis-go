package main

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"
)

type List struct {
	root *Node
	tail *Node
	len  int
}

type Node struct {
	v    []byte
	next *Node
	prev *Node
}

type storageValue struct {
	v         []byte
	expiresAt time.Time
}

type InMemory struct {
	mu         sync.RWMutex
	st         map[string]any
	blpopQueue map[string][]chan []byte
}

func NewInMemory() *InMemory {
	return &InMemory{
		st:         make(map[string]any),
		blpopQueue: make(map[string][]chan []byte),
	}
}

func (im *InMemory) store(k string, v []byte, ttl *time.Duration) {
	im.mu.Lock()
	defer im.mu.Unlock()

	sv := storageValue{v: v}
	if ttl != nil {
		sv.expiresAt = time.Now().Add(*ttl)
	}

	im.st[k] = sv
}

func (im *InMemory) get(k string) ([]byte, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	sv, ok, err := getLive[storageValue](im, k)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, nil
	}

	return sv.v, nil
}

func (im *InMemory) increase(k string) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	sv, ok, err := getLive[storageValue](im, k)
	if err != nil {
		return -1, err
	}

	if !ok {
		im.st[k] = storageValue{v: []byte{'1'}}
		return 1, nil
	}

	n, err := strconv.Atoi(string(sv.v))
	if err != nil {
		return -1, ErrInvalidInt
	}

	sn := strconv.Itoa(n + 1)
	sv.v = []byte(sn)
	im.st[k] = sv

	return n + 1, nil
}

func (im *InMemory) rpush(k string, v [][]byte) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[List](im, k)
	if err != nil {
		return -1, err
	}

	if !ok {
		l = initList(v)
	} else {
		for _, bs := range v {
			insertNode(&l, bs)
		}
	}
	im.st[k] = l
	n := l.len

	im.sendAll(k)

	return n, nil
}

func (im *InMemory) lrange(k string, start int, end int) ([][]byte, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	l, ok, err := getLive[List](im, k)
	if err != nil {
		return nil, err
	}

	res := make([][]byte, 0)
	if !ok {
		return res, nil
	}

	if start < 0 {
		start = max(0, l.len+start)
	}
	if end < 0 {
		end += l.len
	}
	if end >= l.len {
		end = l.len - 1
	}

	if start > end {
		return res, nil
	}

	curr := l.root
	for i := 0; i < start; i++ {
		curr = curr.next
	}

	for i := start; i <= end; i++ {
		res = append(res, curr.v)
		curr = curr.next
	}

	return res, nil
}

func (im *InMemory) lpush(k string, v [][]byte) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[List](im, k)
	if err != nil {
		return -1, err
	}

	if !ok {
		slices.Reverse(v)
		l = initList(v)
	} else {
		for _, bs := range v {
			prependNode(&l, bs)
		}
	}
	im.st[k] = l
	n := l.len

	im.sendAll(k)

	return n, nil
}

func (im *InMemory) llen(k string) (int, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	l, ok, err := getLive[List](im, k)
	if err != nil {
		return -1, err
	}

	if !ok {
		return 0, nil
	}

	return l.len, nil
}

func (im *InMemory) popFront(k string, l List) []byte {
	res := l.root.v
	if l.root.next == nil {
		delete(im.st, k)
		return res
	}

	l.root = l.root.next
	l.root.prev = nil
	l.len -= 1
	im.st[k] = l

	return res
}

func (im *InMemory) lpop(k string) ([]byte, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[List](im, k)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, nil
	}

	return im.popFront(k, l), nil
}

func (im *InMemory) lpopN(k string, n int) ([][]byte, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[List](im, k)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, nil
	}

	res := make([][]byte, 0)
	for range n {
		res = append(res, l.root.v)

		if l.root.next == nil {
			delete(im.st, k)
			return res, nil
		}

		l.root = l.root.next
		l.root.prev = nil
		l.len -= 1
	}
	im.st[k] = l

	return res, nil
}

func (im *InMemory) lpopOrEnqueue(k string) ([]byte, chan []byte, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[List](im, k)
	if err != nil {
		return nil, nil, err
	}

	if ok {
		return im.popFront(k, l), nil, nil
	}

	ch := make(chan []byte, 1)
	im.blpopQueue[k] = append(im.blpopQueue[k], ch)

	return nil, ch, nil
}

func (im *InMemory) bdequeue(k string, ch chan []byte) bool {
	im.mu.Lock()
	defer im.mu.Unlock()

	return im.dequeue(k, ch)
}

func (im *InMemory) blpop(ctx context.Context, k string, ttl time.Duration) ([]byte, error) {
	res, ch, err := im.lpopOrEnqueue(k)
	if res != nil || err != nil {
		return res, err
	}

	if ttl != 0 {
		time.AfterFunc(ttl, func() {
			won := im.bdequeue(k, ch)
			if won {
				ch <- nil
			}
		})
	}

	select {
	case res = <-ch:
		return res, nil
	case <-ctx.Done():
		if im.bdequeue(k, ch) {
			return nil, nil
		}
		return <-ch, nil
	}
}

func (im *InMemory) sendAll(k string) {
	i := 0
	for i < len(im.blpopQueue[k]) {
		l, ok, _ := getLive[List](im, k)
		if !ok {
			break
		}

		res := im.popFront(k, l)
		ch := im.blpopQueue[k][i]
		ch <- res

		i += 1
	}

	if i == len(im.blpopQueue[k]) {
		delete(im.blpopQueue, k)
	} else {
		im.blpopQueue[k] = slices.Delete(im.blpopQueue[k], 0, i)
	}
}

func (im *InMemory) dequeue(k string, ch chan []byte) bool {
	q := im.blpopQueue[k]
	for i, c := range q {
		if c == ch {
			if len(im.blpopQueue[k]) == 1 {
				delete(im.blpopQueue, k)
			} else {
				im.blpopQueue[k] = slices.Delete(im.blpopQueue[k], i, i+1)
			}

			return true
		}
	}
	return false
}

func getLive[T any](im *InMemory, k string) (T, bool, error) {
	v, ok := im.st[k]
	var zero T
	if !ok {
		return zero, false, nil
	}

	if sv, isStr := v.(storageValue); isStr && isExpired(sv.expiresAt) {
		return zero, false, nil
	}

	t, ok2 := v.(T)
	if !ok2 {
		return zero, false, ErrWrongType
	}

	return t, true, nil
}

func initList(v [][]byte) List {
	node := &Node{v: v[0]}
	l := List{
		root: node,
		tail: node,
		len:  1,
	}

	for _, bs := range v[1:] {
		insertNode(&l, bs)
	}

	return l
}

func insertNode(l *List, v []byte) {
	node := &Node{
		v:    v,
		prev: l.tail,
	}
	l.tail.next = node
	l.tail = node
	l.len += 1
}

func prependNode(l *List, v []byte) {
	node := &Node{
		v:    v,
		next: l.root,
	}
	l.root.prev = node
	l.root = node
	l.len += 1
}

func isExpired(t time.Time) bool {
	if t.IsZero() {
		return false
	}

	return t.Before(time.Now())
}
