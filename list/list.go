package list

import "github.com/chechenitza/redis-go/app/common"

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

func New() *List {
	return &List{
		root: nil,
		tail: nil,
		len:  0,
	}
}

func (l *List) Append(v []byte) {
	node := &Node{
		v:    v,
		prev: l.tail,
	}

	if l.tail != nil {
		l.tail.next = node
	}
	if l.root == nil {
		l.root = node
	}

	l.tail = node
	l.len += 1
}

func (l *List) Prepend(v []byte) {
	node := &Node{
		v:    v,
		next: l.root,
	}

	if l.root != nil {
		l.root.prev = node
	}
	if l.tail == nil {
		l.tail = node
	}

	l.root = node
	l.len += 1
}

func (l *List) Len() int {
	return l.len
}

func (l *List) Range(start, end int) ([][]byte, error) {
	start, end, err := common.NormalizeRange(start, end, l.len)
	if err != nil {
		return nil, err
	}

	curr := l.root
	for i := 0; i < start; i++ {
		curr = curr.next
	}

	res := make([][]byte, 0, end-start+1)
	for i := start; i <= end; i++ {
		res = append(res, curr.v)
		curr = curr.next
	}

	return res, nil
}

func (l *List) PopFront() ([]byte, error) {
	if l.root == nil {
		return nil, common.ErrRemoveFromEmpty
	}

	res := l.root.v
	l.len -= 1
	if next := l.root.next; next != nil {
		l.root = next
		next.prev = nil
	}

	return res, nil
}

func NormalizeRange(i, j int, length int) (int, int, error) {
	start, end := i, j
	if start < 0 {
		start = max(0, length+start)
	}
	if end < 0 {
		end += length
	}
	if end >= length {
		end = length - 1
	}

	if start > end {
		return 0, 0, common.ErrInvalidRange
	}

	return start, end, nil
}
