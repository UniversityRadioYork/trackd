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
		wg.Add(1)
		defer wg.Done()
	}

	responses := make(chan []string)
	reply := make(chan bool)

	handle := ClientHandle{
		Broadcast: responses,
	}

	// Tell the client pool we've arrived, and how to contact us.
	cp.Changes <- ClientChange{&handle, true, reply}
	if r := <-reply; !r {
		log.Println("connection refused by client pool")
		return
	}

	go handleConnectionRead(conn, requests, responses, wg)
	handleConnectionWrite(conn, responses)

	// If we get here, the write loop has closed.
	// This only happens if the responses channel is dead, which is either
	// from the client pool or the read loop closing it.

	// Tell the client pool we're off.
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

func handleConnectionWrite(conn net.Conn, responses <-chan []string) {
	for {
		select {
		case response, more := <-responses:
			if !more {
				return
			}
			writeResponse(conn, response)
		}
	}
}

func handleConnectionRead(conn net.Conn, requests chan<- *Request, responses chan<- []string, wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}

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
			requests <- &Request{
				contents: line,
				response: responses,
			}
		}
	}
}

// TODO(CaptainHayashi): use messages, not []string
func writeResponse(conn net.Conn, line []string) error {
	if len(line) == 0 {
		return nil
	}

	bytes, err := baps3.Pack(line[0], line[1:])
	if err != nil {
		return err
	}
	if _, err := conn.Write(bytes); err != nil {
		return err
	}
	return nil
}

// TODO(CaptainHayashi): used?
func outputAck(conn net.Conn, ack string, lstr string, line []string) (err error) {
	tmsg := baps3.NewMessage(baps3.RsAck).AddArg(ack).AddArg(lstr)
	for _, arg := range line {
		tmsg.AddArg(arg)
	}

	tpack, err := tmsg.Pack()
	if err != nil {
		return
	}
	_, err = conn.Write(tpack)

	return
}

func emitRes(output chan<- []string, urlstub string, restype string, resname string, resval string) {
	output <- []string{"RES", urlstub + resname, restype, resval}
	//tmsg := baps3.NewMessage(baps3.RsRes).AddArg(urlstub + resname).AddArg(restype).AddArg(resval)
	//tpack, err := tmsg.Pack()
	//if err != nil {
	//	log.Fatal(err)
	//}
	//writer.Write(tpack)
}
