package main

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    InArray
		wantErr bool
	}{
		{
			name:  "echo cmd",
			input: "*2\r\n$4\r\nECHO\r\n$5\r\nmango\r\n",
			want:  InArray{BulkString("ECHO"), BulkString("mango")},
		},
		{
			name:  "ping command",
			input: "*1\r\n$4\r\nPING\r\n",
			want:  InArray{BulkString("PING")},
		},
		{
			name:    "zero elements rejected",
			input:   "*0\r\n",
			wantErr: true,
		},
		{
			name:    "missing star prefix",
			input:   "$4\r\nPING\r\n",
			wantErr: true,
		},
		{
			name:  "idk some bug",
			input: "*4\r\n$6\r\nLRANGE\r\n$5\r\nmango\r\n$1\r\n0\r\n$2\r\n-1\r\n",
			want:  InArray{BulkString("LRANGE"), BulkString("mango"), BulkString("0"), BulkString("-1")},
		},
	}

	readerWrappers := map[string]func(io.Reader) io.Reader{
		"whole reads":        func(r io.Reader) io.Reader { return r },
		"one byte at a time": iotest.OneByteReader,
		"half reads":         iotest.HalfReader,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for rname, rfunc := range readerWrappers {
				t.Run(rname, func(t *testing.T) {
					r := bufio.NewReader(rfunc(strings.NewReader(tc.input)))
					got, err := Decode(r)
					if tc.wantErr {
						if err == nil {
							t.Fatalf("expected error, got none")
						}
						return
					}
					if err != nil {
						t.Fatalf("unexpected: %v", err)
					}
					if len(got) != len(tc.want) {
						t.Errorf("got len %d, want len %d", len(got), len(tc.want))
					}
					for i := range got {
						if string(got[i]) != string(tc.want[i]) {
							t.Errorf("in array iteration got %s, want %s", got[i], tc.want[i])
						}
					}
				})
			}
		})
	}
}
