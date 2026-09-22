package main

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/chechenitza/redis-go/app/list"
	sset "github.com/chechenitza/redis-go/app/sortedset"
	str "github.com/chechenitza/redis-go/app/string"
)

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

	s := str.New(v)
	if ttl != nil {
		s.SetTTL(*ttl)
	}

	im.st[k] = s
}

func (im *InMemory) get(k string) ([]byte, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	s, ok, err := getLive[*str.String](im, k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	res, ok := s.Get()
	if !ok {
		return nil, nil
	}

	return res, nil
}

func (im *InMemory) increase(k string) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	s, ok, err := getLive[*str.String](im, k)
	if err != nil {
		return -1, err
	}

	if !ok {
		im.st[k] = str.New([]byte("1"))
		return 1, nil
	}

	res, ok := s.Get()
	if !ok {
		s.Set([]byte("1"))
		return 1, nil
	}

	n, err := strconv.Atoi(string(res))
	if err != nil {
		return -1, ErrInvalidInt
	}

	nStr := strconv.Itoa(n + 1)
	s.Set([]byte(nStr))

	return n + 1, nil
}

func (im *InMemory) rpush(k string, v [][]byte) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[*list.List](im, k)
	if err != nil {
		return -1, err
	}

	if !ok {
		l = list.New()
		im.st[k] = l
	}

	for _, bs := range v {
		l.Append(bs)
	}
	n := l.Len()

	im.sendAll(k)

	return n, nil
}

func (im *InMemory) lrange(k string, start int, end int) ([][]byte, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	l, ok, err := getLive[*list.List](im, k)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}

	return l.Range(start, end)
}

func (im *InMemory) lpush(k string, v [][]byte) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[*list.List](im, k)
	if err != nil {
		return -1, err
	}
	if !ok {
		l = list.New()
		im.st[k] = l
	}

	for _, bs := range v {
		l.Prepend(bs)
	}
	n := l.Len()

	im.sendAll(k)

	return n, nil
}

func (im *InMemory) llen(k string) (int, error) {
	im.mu.RLock()
	defer im.mu.RUnlock()

	l, ok, err := getLive[*list.List](im, k)
	if err != nil {
		return -1, err
	}

	if !ok {
		return 0, nil
	}

	return l.Len(), nil
}

func (im *InMemory) lpop(k string) ([]byte, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[*list.List](im, k)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, nil
	}

	res, err := l.PopFront()

	if l.Len() == 0 {
		delete(im.st, k)
	}

	return res, err
}

func (im *InMemory) lpopN(k string, n int) ([][]byte, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[*list.List](im, k)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, nil
	}

	count := min(n, l.Len())
	res := make([][]byte, 0, count)
	for range count {
		v, err := l.PopFront()
		if err != nil {
			break
		}

		res = append(res, v)
	}

	if l.Len() == 0 {
		delete(im.st, k)
	}

	return res, nil
}

func (im *InMemory) lpopOrEnqueue(k string) ([]byte, chan []byte, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	l, ok, err := getLive[*list.List](im, k)
	if err != nil {
		return nil, nil, err
	}

	if ok {
		res, err := l.PopFront()
		if l.Len() == 0 {
			delete(im.st, k)
		}

		return res, nil, err
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
		l, ok, _ := getLive[*list.List](im, k)
		if !ok {
			break
		}

		res, err := l.PopFront()
		if l.Len() == 0 {
			delete(im.st, k)
		}
		if err != nil {
			break
		}
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

func (im *InMemory) zadd(k string, score float64, name string) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	ss, exists, err := getLive[sset.SortedSet](im, k)
	if err != nil {
		return 0, err
	}

	if !exists {
		ss = sset.New()
		im.st[k] = ss
	}

	return ss.Upsert(name, score)
}

func (im *InMemory) zrank(key string, name string) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	ss, exists, err := getLive[sset.SortedSet](im, key)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrNotFound
	}

	return ss.SearchRank(name)
}

func (im *InMemory) zrange(key string, i, j int) ([]string, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	ss, exists, err := getLive[sset.SortedSet](im, key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}

	return ss.Range(i, j)
}

func (im *InMemory) zcard(key string) (int, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	ss, exists, err := getLive[sset.SortedSet](im, key)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrNotFound
	}

	return ss.Len(), nil
}

func (im *InMemory) zscore(key, member string) (float64, error) {
	im.mu.Lock()
	defer im.mu.Unlock()

	ss, exists, err := getLive[sset.SortedSet](im, key)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrNotFound
	}

	return ss.SearchByKey(member)
}

func (im *InMemory) zrem(key, member string) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	ss, exists, err := getLive[sset.SortedSet](im, key)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}

	return ss.Remove(member)
}

func getLive[T any](im *InMemory, k string) (T, bool, error) {
	v, ok := im.st[k]
	var zero T
	if !ok {
		return zero, false, nil
	}

	t, ok2 := v.(T)
	if !ok2 {
		return zero, false, ErrWrongType
	}

	return t, true, nil
}
