package main

import (
	"slices"
	"sync"
)

type Message struct {
	channel string
	v       []byte
}

type Subscriber struct {
	ch   chan Message
	dead chan struct{}
	once sync.Once
}

func (s *Subscriber) kill() {
	s.once.Do(func() {
		close(s.dead)
	})
}

type PubSub struct {
	channels map[string][]*Subscriber
	mu       sync.Mutex
}

func NewPubSub() *PubSub {
	return &PubSub{
		channels: make(map[string][]*Subscriber),
	}
}

func (ps *PubSub) subscribe(channelName string, sub *Subscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	select {
	case <-sub.dead:
		return
	default:
	}

	channel, ok := ps.channels[channelName]
	if !ok {
		channel = make([]*Subscriber, 0)
	}

	if slices.Contains(channel, sub) {
		return
	}

	channel = append(channel, sub)
	ps.channels[channelName] = channel
}

func (ps *PubSub) getSubscribers(channelName string) ([]*Subscriber, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	channel, ok := ps.channels[channelName]
	if !ok {
		return nil, ErrNoChannel
	}

	frozen := make([]*Subscriber, len(channel))
	copy(frozen, channel)

	return frozen, nil
}

func (ps *PubSub) publish(channelName string, msg []byte) (int, error) {
	subs, err := ps.getSubscribers(channelName)
	if err != nil {
		return 0, err
	}

	slow := make([]*Subscriber, 0)
	for _, sub := range subs {
		select {
		case sub.ch <- Message{channel: channelName, v: msg}:
		default:
			slow = append(slow, sub)
		}
	}

	for _, sub := range slow {
		sub.kill()
		ps.unsubscribeFromAll(sub)
	}

	return len(subs) - len(slow), nil
}

func (ps *PubSub) unsubscribe(channelName string, sub *Subscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	channel, ok := ps.channels[channelName]
	if !ok {
		return
	}

	i := slices.Index(channel, sub)
	if i == -1 {
		return
	}

	ps.channels[channelName] = slices.Delete(channel, i, i+1)
}

func (ps *PubSub) unsubscribeFromAll(sub *Subscriber) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for channelName, channel := range ps.channels {
		i := slices.Index(channel, sub)
		if i != -1 {
			if len(ps.channels[channelName]) == 1 {
				delete(ps.channels, channelName)
			} else {
				ps.channels[channelName] = slices.Delete(channel, i, i+1)
			}
		}
	}
}
