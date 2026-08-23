package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func handle(conn net.Conn, s *InMemory) {
	defer conn.Close()
	defer func() {
		err := recover()
		if err != nil {
			fmt.Println("recovered from: ", err)
		}
	}()

	reader := bufio.NewReader(conn)
	b := NewBackend(s)
	for {
		res, err := b.Interpret(reader)
		if err != nil {
			return
		}

		_, err = conn.Write(res)
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

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			continue
		}

		go handle(conn, s)
	}
}
