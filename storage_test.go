package main

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func bsSlice(strs ...string) [][]byte {
	out := make([][]byte, len(strs))
	for i, s := range strs {
		out[i] = []byte(s)
	}
	return out
}

func bulkStringsEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestLrange(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(im *InMemory) string // returns key
		start   int
		end     int
		want    [][]byte
		wantErr error
	}{
		{
			name:  "missing key",
			setup: func(im *InMemory) string { return "missing" },
			start: 0, end: -1,
			want: bsSlice(),
		},
		{
			name: "wrong type key",
			setup: func(im *InMemory) string {
				im.store("k", []byte("v"), nil)
				return "k"
			},
			start: 0, end: -1,
			wantErr: ErrWrongType,
		},

		// single-element list: ["x"]
		{
			name: "single elem (0,0)",
			setup: func(im *InMemory) string {
				im.rpush("single", bsSlice("x"))
				return "single"
			},
			start: 0, end: 0,
			want: bsSlice("x"),
		},
		{
			name: "single elem (0,-1)",
			setup: func(im *InMemory) string {
				im.rpush("single", bsSlice("x"))
				return "single"
			},
			start: 0, end: -1,
			want: bsSlice("x"),
		},
		{
			name: "single elem (-1,-1)",
			setup: func(im *InMemory) string {
				im.rpush("single", bsSlice("x"))
				return "single"
			},
			start: -1, end: -1,
			want: bsSlice("x"),
		},
		{
			name: "single elem start beyond len",
			setup: func(im *InMemory) string {
				im.rpush("single", bsSlice("x"))
				return "single"
			},
			start: 1, end: 1,
			want: bsSlice(),
		},

		// multi-element list: ["a","b","c","d","e"] (len 5)
		{
			name:  "multi (0,0)",
			setup: multiSetup,
			start: 0, end: 0,
			want: bsSlice("a"),
		},
		{
			name:  "multi (0,2)",
			setup: multiSetup,
			start: 0, end: 2,
			want: bsSlice("a", "b", "c"),
		},
		{
			name:  "multi (1,3)",
			setup: multiSetup,
			start: 1, end: 3,
			want: bsSlice("b", "c", "d"),
		},
		{
			name:  "multi (0,4) whole list",
			setup: multiSetup,
			start: 0, end: 4,
			want: bsSlice("a", "b", "c", "d", "e"),
		},
		{
			name:  "multi start > end",
			setup: multiSetup,
			start: 3, end: 1,
			want: bsSlice(),
		},
		{
			name:  "multi start == end mid-list",
			setup: multiSetup,
			start: 2, end: 2,
			want: bsSlice("c"),
		},
		{
			name:  "multi (0,-1) whole list via negative end",
			setup: multiSetup,
			start: 0, end: -1,
			want: bsSlice("a", "b", "c", "d", "e"),
		},
		{
			name:  "multi (0,-2) drop last",
			setup: multiSetup,
			start: 0, end: -2,
			want: bsSlice("a", "b", "c", "d"),
		},
		{
			name:  "multi (1,-1)",
			setup: multiSetup,
			start: 1, end: -1,
			want: bsSlice("b", "c", "d", "e"),
		},
		{
			name:  "multi negative start (-2,4)",
			setup: multiSetup,
			start: -2, end: 4,
			want: bsSlice("d", "e"),
		},
		{
			name:  "multi negative start (-1,4) last element",
			setup: multiSetup,
			start: -1, end: 4,
			want: bsSlice("e"),
		},
		{
			name:  "multi both negative (-3,-1)",
			setup: multiSetup,
			start: -3, end: -1,
			want: bsSlice("c", "d", "e"),
		},
		{
			name:  "multi both negative (-5,-1) whole list",
			setup: multiSetup,
			start: -5, end: -1,
			want: bsSlice("a", "b", "c", "d", "e"),
		},
		{
			name:  "multi both negative start > end after norm",
			setup: multiSetup,
			start: -2, end: -3,
			want: bsSlice(),
		},
		{
			name:  "multi start very negative clamps to 0",
			setup: multiSetup,
			start: -100, end: 2,
			want: bsSlice("a", "b", "c"),
		},
		{
			// real Redis: start clamps to 0, but end (5-100=-95) stays
			// negative and is NOT clamped up to 0 - start(0) > end(-95)
			// so the result must be empty.
			name:  "multi both very negative -> empty (not clamped-end bug)",
			setup: multiSetup,
			start: -100, end: -100,
			want: bsSlice(),
		},
		{
			// same shape as the LRANGE key 0 -100 case from the earlier
			// bug hunt: end stays deeply negative and must NOT be
			// clamped up to 0, or this incorrectly returns ["a"].
			name:  "multi end very negative -> empty",
			setup: multiSetup,
			start: 0, end: -100,
			want: bsSlice(),
		},
		{
			name:  "multi end beyond len clamps to last index",
			setup: multiSetup,
			start: 0, end: 100,
			want: bsSlice("a", "b", "c", "d", "e"),
		},
		{
			name:  "multi end beyond len, start mid-list",
			setup: multiSetup,
			start: 3, end: 100,
			want: bsSlice("d", "e"),
		},
		{
			name:  "multi start beyond len -> empty",
			setup: multiSetup,
			start: 10, end: 20,
			want: bsSlice(),
		},
		{
			name:  "multi start == len -> empty",
			setup: multiSetup,
			start: 5, end: 5,
			want: bsSlice(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			im := NewInMemory()
			key := tc.setup(im)

			got, err := im.lrange(key, tc.start, tc.end)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !bulkStringsEqual(got, tc.want) {
				t.Fatalf("lrange(%d,%d) = %s, want %v", tc.start, tc.end, got, tc.want)
			}
		})
	}
}

func multiSetup(im *InMemory) string {
	im.rpush("multi", bsSlice("a", "b", "c", "d", "e"))
	return "multi"
}

func TestStoreGet(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("v"), nil)

		got, err := im.get("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, []byte("v")) {
			t.Fatalf("get = %q, want %q", got, "v")
		}
	})

	t.Run("missing key", func(t *testing.T) {
		im := NewInMemory()
		got, err := im.get("missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("get = %q, want nil", got)
		}
	})

	t.Run("ttl in future still live", func(t *testing.T) {
		im := NewInMemory()
		ttl := time.Hour
		im.store("k", []byte("v"), &ttl)

		got, err := im.get("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, []byte("v")) {
			t.Fatalf("get = %q, want %q", got, "v")
		}
	})

	t.Run("ttl in past expires lazily", func(t *testing.T) {
		im := NewInMemory()
		ttl := -time.Second
		im.store("k", []byte("v"), &ttl)

		got, err := im.get("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("get = %q, want nil (expired)", got)
		}
	})

	t.Run("get on list-typed key is wrong type", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("a"))

		_, err := im.get("k")
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
	})

	t.Run("store overwrites list-typed key with string", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("a"))
		im.store("k", []byte("v"), nil)

		got, err := im.get("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, []byte("v")) {
			t.Fatalf("get = %q, want %q", got, "v")
		}
	})
}

func TestIncrease(t *testing.T) {
	t.Run("missing key starts at 1", func(t *testing.T) {
		im := NewInMemory()
		n, err := im.increase("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Fatalf("increase = %d, want 1", n)
		}

		got, err := im.get("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, []byte("1")) {
			t.Fatalf("get after increase = %q, want %q", got, "1")
		}
	})

	t.Run("existing numeric value increments", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("5"), nil)

		n, err := im.increase("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 6 {
			t.Fatalf("increase = %d, want 6", n)
		}
	})

	t.Run("non-numeric value errors", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("abc"), nil)

		_, err := im.increase("k")
		if !errors.Is(err, ErrInvalidInt) {
			t.Fatalf("expected ErrInvalidInt, got %v", err)
		}
	})

	t.Run("list-typed key is wrong type", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("a"))

		_, err := im.increase("k")
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
	})
}

func TestRpushLpush(t *testing.T) {
	t.Run("rpush on missing key creates list in order", func(t *testing.T) {
		im := NewInMemory()
		n, err := im.rpush("k", bsSlice("a", "b"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 2 {
			t.Fatalf("rpush count = %d, want 2", n)
		}

		got, err := im.lrange("k", 0, -1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bulkStringsEqual(got, bsSlice("a", "b")) {
			t.Fatalf("lrange = %v, want [a b]", got)
		}
	})

	t.Run("rpush appends to existing list", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("a", "b"))
		n, err := im.rpush("k", bsSlice("c"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 3 {
			t.Fatalf("rpush count = %d, want 3", n)
		}

		got, _ := im.lrange("k", 0, -1)
		if !bulkStringsEqual(got, bsSlice("a", "b", "c")) {
			t.Fatalf("lrange = %v, want [a b c]", got)
		}
	})

	t.Run("lpush on missing key creates list, last arg ends up at head", func(t *testing.T) {
		im := NewInMemory()
		n, err := im.lpush("k", bsSlice("a", "b"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 2 {
			t.Fatalf("lpush count = %d, want 2", n)
		}

		got, _ := im.lrange("k", 0, -1)
		if !bulkStringsEqual(got, bsSlice("b", "a")) {
			t.Fatalf("lrange = %v, want [b a]", got)
		}
	})

	t.Run("lpush prepends to existing list", func(t *testing.T) {
		im := NewInMemory()
		im.lpush("k", bsSlice("a", "b")) // -> [b a]
		n, err := im.lpush("k", bsSlice("c"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 3 {
			t.Fatalf("lpush count = %d, want 3", n)
		}

		got, _ := im.lrange("k", 0, -1)
		if !bulkStringsEqual(got, bsSlice("c", "b", "a")) {
			t.Fatalf("lrange = %v, want [c b a]", got)
		}
	})

	t.Run("rpush on string-typed key is wrong type", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("v"), nil)

		_, err := im.rpush("k", bsSlice("a"))
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
	})

	t.Run("lpush on string-typed key is wrong type", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("v"), nil)

		_, err := im.lpush("k", bsSlice("a"))
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
	})
}

func TestLlen(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		im := NewInMemory()
		n, err := im.llen("missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 {
			t.Fatalf("llen = %d, want 0", n)
		}
	})

	t.Run("after rpush and lpush", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("a", "b"))
		im.lpush("k", bsSlice("c"))

		n, err := im.llen("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 3 {
			t.Fatalf("llen = %d, want 3", n)
		}
	})

	t.Run("string-typed key is wrong type", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("v"), nil)

		_, err := im.llen("k")
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
	})
}

func TestLpop(t *testing.T) {
	t.Run("missing key", func(t *testing.T) {
		im := NewInMemory()
		got, err := im.lpop("missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("lpop = %q, want nil", got)
		}
	})

	t.Run("pops first element and advances root", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("a", "b", "c"))

		got, err := im.lpop("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, []byte("a")) {
			t.Fatalf("lpop = %q, want %q", got, "a")
		}

		n, err := im.llen("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 2 {
			t.Fatalf("llen after lpop = %d, want 2", n)
		}

		rest, err := im.lrange("k", 0, -1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bulkStringsEqual(rest, bsSlice("b", "c")) {
			t.Fatalf("lrange after lpop = %v, want [b c]", rest)
		}
	})

	t.Run("string-typed key is wrong type", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("v"), nil)

		_, err := im.lpop("k")
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
	})

	t.Run("popping last element deletes the key", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("only"))

		got, err := im.lpop("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(got, []byte("only")) {
			t.Fatalf("lpop = %q, want %q", got, "only")
		}

		n, err := im.llen("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 {
			t.Fatalf("llen after popping last element = %d, want 0", n)
		}

		// key must be gone entirely, not left behind as an empty list -
		// confirm by recreating it via rpush and checking it starts fresh.
		rn, err := im.rpush("k", bsSlice("fresh"))
		if err != nil {
			t.Fatalf("unexpected error recreating key: %v", err)
		}
		if rn != 1 {
			t.Fatalf("rpush count after key deletion = %d, want 1 (stale list left behind)", rn)
		}
	})
}

func TestLpopN(t *testing.T) {
	cases := []struct {
		name          string
		setup         func(im *InMemory) string // returns key
		n             int
		want          [][]byte
		wantRemaining [][]byte
		wantDeleted   bool
	}{
		{
			name:          "n=0 pops nothing, list untouched",
			setup:         multiSetup,
			n:             0,
			want:          bsSlice(),
			wantRemaining: bsSlice("a", "b", "c", "d", "e"),
		},
		{
			name:          "n=1 pops single element",
			setup:         multiSetup,
			n:             1,
			want:          bsSlice("a"),
			wantRemaining: bsSlice("b", "c", "d", "e"),
		},
		{
			name:          "n=3 pops multiple elements in order",
			setup:         multiSetup,
			n:             3,
			want:          bsSlice("a", "b", "c"),
			wantRemaining: bsSlice("d", "e"),
		},
		{
			name:        "n==len pops entire list and deletes key",
			setup:       multiSetup,
			n:           5,
			want:        bsSlice("a", "b", "c", "d", "e"),
			wantDeleted: true,
		},
		{
			name:        "n>len pops all available elements and deletes key",
			setup:       multiSetup,
			n:           10,
			want:        bsSlice("a", "b", "c", "d", "e"),
			wantDeleted: true,
		},
		{
			name: "single element list, n=1 deletes key",
			setup: func(im *InMemory) string {
				im.rpush("single", bsSlice("x"))
				return "single"
			},
			n:           1,
			want:        bsSlice("x"),
			wantDeleted: true,
		},
		{
			name: "single element list, n>len deletes key",
			setup: func(im *InMemory) string {
				im.rpush("single", bsSlice("x"))
				return "single"
			},
			n:           5,
			want:        bsSlice("x"),
			wantDeleted: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			im := NewInMemory()
			key := tc.setup(im)

			got, err := im.lpopN(key, tc.n)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bulkStringsEqual(got, tc.want) {
				t.Fatalf("lpopN(%d) = %v, want %v", tc.n, got, tc.want)
			}

			n, err := im.llen(key)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantDeleted {
				if n != 0 {
					t.Fatalf("llen after lpopN = %d, want 0 (key deleted)", n)
				}
				// key must be gone entirely, not left behind as an empty
				// list - confirm by recreating it and checking it starts fresh.
				rn, err := im.rpush(key, bsSlice("fresh"))
				if err != nil {
					t.Fatalf("unexpected error recreating key: %v", err)
				}
				if rn != 1 {
					t.Fatalf("rpush count after key deletion = %d, want 1 (stale list left behind)", rn)
				}
				return
			}

			if n != len(tc.wantRemaining) {
				t.Fatalf("llen after lpopN = %d, want %d", n, len(tc.wantRemaining))
			}
			rest, err := im.lrange(key, 0, -1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bulkStringsEqual(rest, tc.wantRemaining) {
				t.Fatalf("lrange after lpopN = %v, want %v", rest, tc.wantRemaining)
			}
		})
	}

	t.Run("missing key returns nil, not empty array", func(t *testing.T) {
		im := NewInMemory()
		got, err := im.lpopN("missing", 3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("lpopN = %v, want nil", got)
		}
	})

	t.Run("string-typed key is wrong type", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("v"), nil)

		_, err := im.lpopN("k", 3)
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
	})
}

func TestBlpop(t *testing.T) {
	t.Run("data already present pops immediately, no channel", func(t *testing.T) {
		im := NewInMemory()
		im.rpush("k", bsSlice("a", "b"))

		got, ch, err := im.blpop("k", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch != nil {
			t.Fatalf("expected nil channel, got one (should not block)")
		}
		if !bytes.Equal(got, []byte("a")) {
			t.Fatalf("blpop = %q, want %q", got, "a")
		}
	})

	t.Run("wrong type key errors immediately, no blocking", func(t *testing.T) {
		im := NewInMemory()
		im.store("k", []byte("v"), nil)

		_, ch, err := im.blpop("k", 0)
		if !errors.Is(err, ErrWrongType) {
			t.Fatalf("expected ErrWrongType, got %v", err)
		}
		if ch != nil {
			t.Fatalf("expected nil channel on error")
		}
	})

	t.Run("missing key blocks until rpush delivers", func(t *testing.T) {
		im := NewInMemory()

		v, ch, err := im.blpop("k", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch == nil {
			t.Fatalf("expected non-nil channel to block on")
		}
		if v != nil {
			t.Fatalf("expected nil value alongside a wait channel")
		}

		res := make(chan []byte, 1)
		go func() { res <- <-ch }()

		n, err := im.rpush("k", bsSlice("x"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 1 {
			t.Fatalf("rpush count = %d, want 1", n)
		}

		select {
		case got := <-res:
			if !bytes.Equal(got, []byte("x")) {
				t.Fatalf("blpop delivered %q, want %q", got, "x")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for blpop to receive pushed value")
		}

		// the pushed element was handed straight to the waiter, so the
		// key must be gone entirely, not left behind as an empty list.
		ln, err := im.llen("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ln != 0 {
			t.Fatalf("llen after delivery = %d, want 0", ln)
		}
	})

	t.Run("missing key times out and yields nil", func(t *testing.T) {
		im := NewInMemory()

		_, ch, err := im.blpop("missing", 50*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch == nil {
			t.Fatalf("expected non-nil channel to block on")
		}

		select {
		case got := <-ch:
			if got != nil {
				t.Fatalf("expected nil (timeout), got %q", got)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for blpop's own timeout to fire")
		}
	})

	t.Run("two waiters served in FIFO order by one push", func(t *testing.T) {
		im := NewInMemory()

		_, ch1, err := im.blpop("k", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, ch2, err := im.blpop("k", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res1 := make(chan []byte, 1)
		res2 := make(chan []byte, 1)
		go func() { res1 <- <-ch1 }()
		go func() { res2 <- <-ch2 }()

		n, err := im.rpush("k", bsSlice("first", "second"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 2 {
			t.Fatalf("rpush count = %d, want 2", n)
		}

		select {
		case got := <-res1:
			if !bytes.Equal(got, []byte("first")) {
				t.Fatalf("first waiter got %q, want %q", got, "first")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for first waiter")
		}

		select {
		case got := <-res2:
			if !bytes.Equal(got, []byte("second")) {
				t.Fatalf("second waiter got %q, want %q", got, "second")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for second waiter")
		}

		ln, err := im.llen("k")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ln != 0 {
			t.Fatalf("llen after both deliveries = %d, want 0", ln)
		}
	})

	t.Run("one waiter, push count reflects full push not just consumed element", func(t *testing.T) {
		im := NewInMemory()

		_, ch, err := im.blpop("k", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		res := make(chan []byte, 1)
		go func() { res <- <-ch }()

		n, err := im.rpush("k", bsSlice("x", "y", "z"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 3 {
			t.Fatalf("rpush count = %d, want 3", n)
		}

		select {
		case got := <-res:
			if !bytes.Equal(got, []byte("x")) {
				t.Fatalf("waiter got %q, want %q", got, "x")
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for waiter to receive")
		}

		rest, err := im.lrange("k", 0, -1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bulkStringsEqual(rest, bsSlice("y", "z")) {
			t.Fatalf("lrange after delivery = %v, want [y z]", rest)
		}
	})
}
