//go:build windows

package npipe_test

import (
	// Standard
	"bufio"
	"fmt"
	"net"

	"github.com/Ne0nd0g/npipe"
)

// Use Dial to connect to a server and read messages from it.
func ExampleDial() {
	conn, err := npipe.Dial(`\\.\pipe\mypipe`)
	if err != nil {
		// handle error
	}
	if _, err := fmt.Fprintln(conn, "Hi server!"); err != nil {
		// handle error
	}
	r := bufio.NewReader(conn)
	msg, err := r.ReadString('\n')
	if err != nil {
		// handle error
	}
	fmt.Println(msg)
}

// Use Listen to start a server, and accept connections with Accept().
func ExampleListen() {
	ln, err := npipe.Listen(`\\.\pipe\mypipe`)
	if err != nil {
		// handle error
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			// handle error
			continue
		}

		// handle connection like any other net.Conn
		go func(conn net.Conn) {
			r := bufio.NewReader(conn)
			msg, err := r.ReadString('\n')
			if err != nil {
				// handle error
				return
			}
			fmt.Println(msg)
		}(conn)
	}
}
