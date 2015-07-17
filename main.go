package main

import (
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/UniversityRadioYork/baps3-go"
	"github.com/docopt/docopt-go"
	_ "github.com/lib/pq"
	"gopkg.in/tomb.v2"
)

type Request struct {
	contents *baps3.Message
	response chan<- *baps3.Message
}

func main() {
	usage := `trackd - track resolving server for BAPS3

Usage:
    trackd HOSTPORT

Options:
    HOSTPORT       The host and port on which trackd should listen (host:port).
    -h, --help     Show this message.
    -v, --version  Show version.
`
	arguments, err := docopt.Parse(usage, nil, true, "trackd 0.0", true)
	if err != nil {
		log.Fatal(err)
	}

	db, err := getDB()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	t := NewTrackDB(db, `M:\%d\%d`)

	RunTcpServer(RequestMap{
		baps3.RqRead: func(b, r chan<- *baps3.Message, s []string) (bool, error) { return handleRead(b, r, t, s) },
		baps3.RqQuit: func(_, _ chan<- *baps3.Message, _ []string) (bool, error) { return true, nil },
	}, arguments["HOSTPORT"].(string))
}

// RequestHandler is the type of handlers added to a RequestMap.
type RequestHandler func(chan<- *baps3.Message, chan<- *baps3.Message, []string) (bool, error)

// RequestMap is a map from requests (as message words) to RequestHandlers.
type RequestMap map[baps3.MessageWord]RequestHandler

// RunTcpServer creates and runs a Bifrost server using TCP as a transport.
// It will respond to requests using the functions in requestMap
func RunTcpServer(requestMap RequestMap, hostport string) {
	ln, err := net.Listen("tcp", hostport)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	cpQuit := make(chan struct{})

	var t tomb.Tomb

	clientPoolHandle := NewClientPool(cpQuit)
	t.Go(func() error { return clientPoolHandle.Pool.Run(&t) })

	requests := make(chan *Request)

	t.Go(func() error { return AcceptLoop(ln, requests, &clientPoolHandle, &t) })

	RequestLoop(requests, clientPoolHandle.Broadcast, requestMap, &t)

	t.Killf("main loop closing")

	// To close the accept loop, we have to kill off the acceptor.
	if err := ln.Close(); err != nil {
		log.Fatal(err)
	}

	log.Println(t.Wait())
}

func AcceptLoop(ln net.Listener, requests chan<- *Request, clientPoolHandle *ClientPoolHandle, t *tomb.Tomb) (err error) {
	defer func() { log.Println("accept loop closing") }()

	for {
		conn, cerr := ln.Accept()
		if cerr != nil {
			log.Println(err)
			err = cerr
			break
		}

		t.Go(func() error { return handleConnection(conn, requests, clientPoolHandle, t) })
	}

	return
}

func RequestLoop(requests <-chan *Request, broadcast chan<- *baps3.Message, requestMap RequestMap, t *tomb.Tomb) {
	for {
		select {
		case <-t.Dying():
			return
		case r, more := <-requests:
			if !more {
				return
			}
			log.Printf("received request: %q", r.contents)
			if finished := handleRequest(broadcast, requestMap, r); finished {
				return
			}
		}
	}
}

func handleRequest(broadcast chan<- *baps3.Message, requestMap RequestMap, request *Request) bool {
	var lerr error
	finished := false

	msg := request.contents

	// TODO: handle bad command
	cmdfunc, ok := requestMap[msg.Word()]
	if ok {
		finished, lerr = cmdfunc(broadcast, request.response, msg.Args())
	} else {
		lerr = fmt.Errorf("FIXME: unknown command %q", msg.Word())
	}

	acktype := "???"
	lstr := "Success"
	if lerr == nil {
		acktype = "OK"
	} else {
		// TODO: proper error distinguishment
		acktype = "FAIL"
		lstr = lerr.Error()
	}

	log.Printf("Sending ack: %q, %q", acktype, lstr)

	request.response <- baps3.NewMessage(baps3.RsAck).AddArg(acktype).AddArg(lstr)
	return finished
}

func handleRead(_ chan<- *baps3.Message, response chan<- *baps3.Message, t *TrackDB, args []string) (bool, error) {
	// read TAG(ignored) PATH
	if 2 == len(args) {
		resources := strings.Split(strings.Trim(args[1], "/"), "/")
		if len(resources) == 2 && resources[0] == "tracks" {
			log.Printf("LOOKUP %q", resources[1])
			t.LookupTrack(response, resources[1])
		} else {
			return false, fmt.Errorf("FIXME: unknown read %q", resources)
		}
	} else {
		// TODO: send failure here
		return false, fmt.Errorf("FIXME: bad read %q", args)
	}

	return false, nil
}
