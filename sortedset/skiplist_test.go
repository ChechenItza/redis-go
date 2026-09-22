package sortedset

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
)

func newRNG(t *testing.T) *rand.Rand {
	t.Helper()

	seed := uint64(time.Now().UnixNano())
	t.Logf("seed: SKIPLIST_SEED=%d", seed)

	return rand.New(rand.NewPCG(seed, seed>>32))
}

func TestUpsert(t *testing.T) {
	rng := newRNG(t)
	sl := NewSkipList()

	if len(sl.Levels) != maxLevel {
		t.Fatalf("invalid level count, got %d want %d", len(sl.Levels), maxLevel)
	}

	type input struct {
		key string
		val float64
	}

	casesLen := rng.IntN(50) + 1
	cases := make([]input, 0, casesLen)
	for i := range casesLen {
		key := fmt.Sprintf("test%d", i)
		val := rng.Float64()

		sl.Insert(key, val)
		cases = append(cases, input{key: key, val: val})
	}

	count := 0
	for node := sl.Levels[0]; node != nil; node = node.forward[0] {
		count++
	}
	if count != casesLen {
		t.Errorf("invalid count, got %d want %d\n%s", count, casesLen, dumpSkipList(sl))
	}

	slices.SortFunc(cases, func(a, b input) int {
		return cmp.Compare(a.val, b.val)
	})

	node := sl.Levels[0]
	for i, tcase := range cases {
		if node == nil {
			t.Fatalf("level 0 ended early at index %d\n%s", i, dumpSkipList(sl))
		}
		if tcase.val != node.value {
			t.Errorf("invalid sorting at index %d, got %.4f want %.4f\n%s",
				i, node.value, tcase.val, dumpSkipList(sl))
			break
		}
		node = node.forward[0]
	}

	t.Logf("%s\n", dumpSkipList(sl))
}

func TestRemove(t *testing.T) {
	rng := newRNG(t)
	sl := NewSkipList()

	if len(sl.Levels) != maxLevel {
		t.Fatalf("invalid level count, got %d want %d", len(sl.Levels), maxLevel)
	}

	casesLen := rng.IntN(50) + 1
	cases := make([]*slNode, 0, casesLen)
	for i := range casesLen {
		key := fmt.Sprintf("test%d", i)
		val := rng.Float64()

		node := sl.Insert(key, val)
		cases = append(cases, node)
	}

	t.Logf("%s\n", dumpSkipList(sl))

	for _, tcase := range cases {
		sl.Remove(tcase)
	}

	t.Logf("%s\n", dumpSkipList(sl))
}

func TestRank(t *testing.T) {
	sl := NewSkipList()

	if len(sl.Levels) != maxLevel {
		t.Fatalf("invalid level count, got %d want %d", len(sl.Levels), maxLevel)
	}

	type input struct {
		key string
		val float64
	}

	casesLen := 4
	cases := make([]input, 0, casesLen)
	cases = append(cases, input{key: "pear", val: 12.221113714988885})
	cases = append(cases, input{key: "mango", val: 12.221113714988885})
	cases = append(cases, input{key: "blueberrry", val: 36.804537294846824})
	cases = append(cases, input{key: "banana", val: 15.162568783833144})
	for _, in := range cases {
		sl.Insert(in.key, in.val)
	}

	count := 0
	for node := sl.Levels[0]; node != nil; node = node.forward[0] {
		count++
	}
	if count != casesLen {
		t.Errorf("invalid count, got %d want %d\n%s", count, casesLen, dumpSkipList(sl))
	}

	slices.SortFunc(cases, func(a, b input) int {
		if cmpRes := cmp.Compare(a.val, b.val); cmpRes != 0 {
			return cmpRes
		}

		if a.key < b.key {
			return -1
		}
		return 1
	})

	for i, tcase := range cases {
		rank, err := sl.SearchRank(tcase.key)
		if err != nil {
			t.Errorf("got error %v", err)
		}

		if i != rank {
			t.Errorf("key: %s; expected %d, got %d", tcase.key, i, rank)
		}
	}

	t.Logf("%s\n", dumpSkipList(sl))
}

func dumpSkipList(sl *skipList) string {
	var b strings.Builder
	for i := len(sl.Levels) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "Lvl %d: ", i)
		for node := sl.Levels[i]; node != nil; node = node.forward[i] {
			fmt.Fprintf(&b, "{%s, %.2f} -> ", node.key, node.value)
		}
		b.WriteString("nil\n")
	}
	return b.String()
}
