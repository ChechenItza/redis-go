package common

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
		return 0, 0, ErrInvalidRange
	}

	return start, end, nil
}
