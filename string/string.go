package string

import (
	"time"
)

type String struct {
	v         []byte
	expiresAt time.Time
}

func New(v []byte) *String {
	return &String{
		v: v,
	}
}

func (s *String) SetTTL(ttl time.Duration) {
	s.expiresAt = time.Now().Add(ttl)
}

func (s *String) Set(v []byte) {
	s.v = v
	s.expiresAt = time.Time{}
}

func (s *String) Get() ([]byte, bool) {
	if isExpired(s.expiresAt) {
		s.v = nil
		return nil, false
	}
	return s.v, true
}

func isExpired(t time.Time) bool {
	if t.IsZero() {
		return false
	}

	return t.Before(time.Now())
}
