package main

import (
	"io"
	"log"
	"net"
	"sync"

	"github.com/UniversityRadioYork/baps3-go"
)

func handleConnection(conn net.Conn, requests chan<- *Request, cp *ClientPoolHandle, wg *sync.WaitGroup) {
	// Make sure the main server waits for this connection to close
	// properly.  This only handles this routine: handleConnectionRead has
	// its own waitgroup handling.
	if wg != nil {
		defer wg.Done()
	}

	defer func() { log.Println("connection write side closing") }()

	responses := make(chan *baps3.Message)
	disconnect := make(chan struct{})
	reply := make(chan bool)

	handle := ClientHandle{
		Broadcast:  responses,
		Disconnect: disconnect,
	}

	// Tell the client pool we've arrived, and how to contact us.
	cp.Changes <- ClientChange{&handle, true, reply}
	if r := <-reply; !r {
		log.Println("connection refused by client pool")
		return
	}

	go handleConnectionRead(conn, requests, responses, wg)
	handleConnectionWrite(conn, responses, disconnect)

	// If we get here, the write loop has closed.
	// This only happens if the responses channel is dead, which is either
	// from the client pool or the read loop closing it.

	// Tell the client pool we're off.
	log.Println("connection write side signalling closure")
	cp.Changes <- ClientChange{&handle, false, reply}
	if r := <-reply; !r {
		log.Println("connection removal refused by client pool")
		return
	}

	// Now close the actual connection.
	if err := conn.Close(); err != nil {
		log.Printf("couldn't close connection: %q", err)
	}

}

func handleConnectionWrite(conn net.Conn, responses <-chan *baps3.Message, quit <-chan struct{}) {
	for {
		select {
		case <-quit:
			log.Println("connection has quit signal")
			// The write routine has responsibility for listening
			// to the server quit channel to see if the connection
			// must be closed.
			return
		case response, more := <-responses:
			if !more {
				return
			}
			writeResponse(conn, response)
		}
	}
}

func handleConnectionRead(conn net.Conn, requests chan<- *Request, responses chan<- *baps3.Message, wg *sync.WaitGroup) {
	if wg != nil {
		defer wg.Done()
	}
	defer func() { log.Println("connection read side closing") }()

	// Ensure the write portion is closed when reading stops.
	// The closing of the responses channel will do this.
	// Note that the requests channel is later closed when the writing
	// section stops.
	defer func() {
		close(responses)
	}()

	buf := make([]byte, 1024)
	tok := baps3.NewTokeniser()

	for {
		nbytes, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				// TODO: handle error correctly, send error to client
				log.Printf("connection read error: %q", err)
			}
			return
		}

		lines, _, err := tok.Tokenise(buf[:nbytes])
		if err != nil {
			// TODO: handle error correctly, retry tokenising perhaps
			log.Printf("connection tokenise error: %q", err)
			return
		}

		for _, line := range lines {
			msg, err := baps3.LineToMessage(line)
			if err != nil {
				log.Printf("bad message: %q", line)
			} else {
				requests <- &Request{
					contents: msg,
					response: responses,
				}
			}
		}
	}
}

func writeResponse(conn net.Conn, message *baps3.Message) error {
	bytes, err := message.Pack()
	if err != nil {
		return err
	}
	if _, err := conn.Write(bytes); err != nil {
		return err
	}
	return nil
}
