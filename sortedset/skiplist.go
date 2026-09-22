package sortedset

import (
	"math/rand/v2"

	"github.com/chechenitza/redis-go/app/common"
)

const maxLevel = 4

type skipList struct {
	Levels []*slNode
}

func NewSkipList() *skipList {
	return &skipList{
		Levels: make([]*slNode, maxLevel),
	}
}

type slNode struct {
	forward []*slNode
	back    []*slNode
	key     string
	value   float64
}

func (sl *skipList) Search(val float64) (string, error) {
	lvl := len(sl.Levels) - 1
	if lvl < 0 {
		return "", common.ErrNotFound
	}

	prev := sl.Levels[lvl]
	node := sl.Levels[lvl]
	for node != nil {
		if node.value == val {
			return node.key, nil
		}

		if node.value > val || node.forward[lvl] == nil {
			lvl -= 1
			if lvl < 0 {
				break
			}

			node = prev.forward[lvl]
			continue
		}

		prev = node
		node = node.forward[lvl]
	}

	return "", common.ErrNotFound
}

func (sl *skipList) SearchRank(key string) (int, error) {
	curr := sl.Levels[0]
	for i := 0; curr != nil; i++ {
		if curr.key == key {
			return i, nil
		}

		curr = curr.forward[0]
	}

	return 0, common.ErrNotFound
}

func (sl *skipList) Range(start, end int) ([]string, error) {
	curr := sl.Levels[0]
	for i := 0; curr != nil && i != start; i++ {
		curr = curr.forward[0]
	}

	res := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		res = append(res, curr.key)

		curr = curr.forward[0]
	}

	return res, nil
}

func (sl *skipList) Insert(key string, val float64) *slNode {
	fwd := make([]*slNode, 4)
	back := make([]*slNode, 4)
	node := &slNode{
		forward: fwd,
		back:    back,
		key:     key,
		value:   val,
	}

	lvl := 0
	for ; ; lvl++ {
		curr := sl.Levels[lvl]
		if curr == nil {
			sl.Levels[lvl] = node
		} else {
			sl.insertInLevel(node, lvl)
		}

		if rand.Float64() > 0.25 || lvl+1 == maxLevel {
			break
		}
	}

	return node
}

func (sl *skipList) Remove(node *slNode) {
	for lvl := len(sl.Levels) - 1; lvl >= 0; lvl-- {
		if node.back[lvl] == nil && node.forward[lvl] == nil {
			if sl.Levels[lvl] == node {
				sl.Levels[lvl] = nil
			}
			continue
		}

		if node.back[lvl] == nil && node.forward[lvl] != nil {
			next := node.forward[lvl]
			next.back[lvl] = nil
			sl.Levels[lvl] = next
			continue
		}

		if node.back[lvl] != nil && node.forward[lvl] == nil {
			prev := node.back[lvl]
			prev.forward[lvl] = nil
			continue
		}

		prev := node.back[lvl]
		next := node.forward[lvl]
		prev.forward[lvl] = next
		next.back[lvl] = prev
	}
}

func (sl *skipList) insertInLevel(node *slNode, lvl int) {
	curr := sl.Levels[lvl]
	if canInsert(curr, node) {
		sl.Levels[lvl] = node
		node.forward[lvl] = curr
		curr.back[lvl] = node
		return
	}

	for ; curr != nil; curr = curr.forward[lvl] {
		if curr.forward[lvl] == nil ||
			canInsert(curr.forward[lvl], node) {
			break
		}
	}

	next := curr.forward[lvl]
	curr.forward[lvl] = node
	node.forward[lvl] = next
	node.back[lvl] = curr
	if next != nil {
		next.back[lvl] = node
	}
}

func canInsert(curr *slNode, node *slNode) bool {
	return node.value < curr.value ||
		(node.value == curr.value && node.key < curr.key)
}
