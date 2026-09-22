package sortedset

import "github.com/chechenitza/redis-go/app/common"

type SortedSet struct {
	sl *skipList
	m  map[string]*slNode
}

func New() SortedSet {
	return SortedSet{
		sl: NewSkipList(),
		m:  make(map[string]*slNode),
	}
}

func (ss SortedSet) Upsert(key string, val float64) (int, error) {
	affected := 1
	if node, ok := ss.m[key]; ok {
		affected = 0
		ss.sl.Remove(node)
	}

	node := ss.sl.Insert(key, val)
	ss.m[key] = node
	return affected, nil
}

func (ss SortedSet) SearchByKey(key string) (float64, error) {
	if node, ok := ss.m[key]; ok {
		return node.value, nil
	}

	return 0, common.ErrNotFound
}

func (ss SortedSet) SearchByVal(val float64) (string, error) {
	return ss.sl.Search(val)
}

func (ss SortedSet) SearchRank(key string) (int, error) {
	return ss.sl.SearchRank(key)
}

func (ss SortedSet) Range(start, end int) ([]string, error) {
	start, end, err := common.NormalizeRange(start, end, len(ss.m))
	if err != nil {
		return nil, common.ErrInvalidRange
	}

	return ss.sl.Range(start, end)
}

func (ss SortedSet) Remove(member string) error {
	node, ok := ss.m[member]
	if !ok {
		return common.ErrNotFound
	}

	ss.sl.Remove(node)
	delete(ss.m, member)

	return nil
}

func (ss SortedSet) Len() int {
	return len(ss.m)
}
