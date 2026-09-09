package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
)

type Response struct {
	v   []byte
	err error
}

func safely(cancel context.CancelFunc, fn func()) {
	defer cancel()
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("recovered from: ", err)
		}
	}()

	fn()
}

func handle(ctx context.Context, conn net.Conn, s *InMemory, ps *PubSub) {
	ctx, cancel := context.WithCancel(ctx)
	cs := NewConnState(ctx, s, ps)

	defer conn.Close()
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("recovered from: ", err)
		}
	}()
	defer cancel()
	defer cs.Teardown()

	reader := bufio.NewReader(conn)

	resCh := make(chan Response)
	go safely(cancel, func() {
		for {
			res, err := cs.Interpret(reader)
			select {
			case resCh <- Response{v: res, err: err}:
			case <-ctx.Done():
				return
			}
		}
	})
	go safely(cancel, func() {
		for {
			res, err := cs.Listen()
			select {
			case resCh <- Response{v: res, err: err}:
			case <-ctx.Done():
				return
			}
		}
	})

	for {
		var res Response
		select {
		case res = <-resCh:
		case <-ctx.Done():
			return
		}

		if res.err != nil {
			return
		}

		_, err := conn.Write(res.v)
		if err != nil {
			fmt.Println("Error writing message: ", err.Error())
			return
		}
	}
}

func main() {
	fmt.Println("Logs from your program will appear here!")

	l, err := net.Listen("tcp", "0.0.0.0:6379")
	if err != nil {
		fmt.Println("Failed to bind to port 6379")
		os.Exit(1)
	}

	s := NewInMemory()
	ps := NewPubSub()
	ctx := context.Background()

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			continue
		}

		go handle(ctx, conn, s, ps)
	}
}
