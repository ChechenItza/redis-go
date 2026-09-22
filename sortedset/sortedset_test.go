package sortedset

import (
	"cmp"
	"slices"
	"testing"
)

func TestRange(t *testing.T) {
	ss := New()

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
		ss.Upsert(in.key, in.val)
	}

	if len(ss.m) != casesLen {
		t.Errorf("invalid count, got %d want %d\n%s", len(ss.m), casesLen, dumpSkipList(ss.sl))
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

	names, err := ss.Range(-2, -1)
	if err != nil {
		t.Errorf("got error %v", err)
	}

	t.Logf("%v", names)

	t.Logf("%s\n", dumpSkipList(ss.sl))
}
