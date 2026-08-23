package main

import (
	"errors"
	"fmt"
)

var (
	ErrParsing       = errors.New("invalid input")
	ErrBadPrefix     = fmt.Errorf("expected different prefix: %w", ErrParsing)
	ErrNegativeCount = fmt.Errorf("incorrect count: %w", ErrParsing)
	ErrBadLength     = fmt.Errorf("unparsable length: %w", ErrParsing)
	ErrInvalidInt    = fmt.Errorf("value is not an integer or out of range")
	ErrWrongType     = fmt.Errorf("incorrect type")
)
