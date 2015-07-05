package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/docopt/docopt-go"
	_ "github.com/lib/pq"
)

type Request struct {
	contents []string
	response chan<- []string
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

	ln, err := net.Listen("tcp", arguments["HOSTPORT"].(string))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	var wg sync.WaitGroup

	clientPoolHandle := NewClientPool()
	go clientPoolHandle.Pool.Run(&wg)

	requests := make(chan *Request)
	go RequestLoop(ln, requests, clientPoolHandle.Broadcast, &wg)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConnection(conn, requests, &clientPoolHandle, &wg)
	}

	wg.Wait()
}

func RequestLoop(ln io.Closer, requests <-chan *Request, broadcast chan<- []string, wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}

	db, err := getDB()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			log.Fatal(err)
		}
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}()
	t := NewTrackDB(db, `M:\%d\%d`)

	for {
		select {
		case r, more := <-requests:
			if !more {
				return
			}
			log.Printf("received request: %q", r.contents)
			if finished := handleRequest(broadcast, t, r); finished {
				return
			}
		}
	}
}

type CmdTable map[string]func(chan<- []string, chan<- []string, *TrackDB, []string) (bool, error)

var cmds CmdTable = CmdTable{
	"read": handleRead,
	"quit": handleQuit,
}

func handleQuit(_, _ chan<- []string, _ *TrackDB, _ []string) (bool, error) {
	return true, nil
}

func handleRequest(broadcast chan<- []string, t *TrackDB, request *Request) bool {
	var lerr error
	finished := false

	line := request.contents

	// TODO: handle quit
	// TODO: handle bad command
	if 0 < len(line) {
		cmdfunc, ok := cmds[line[0]]
		if ok {
			finished, lerr = cmdfunc(broadcast, request.response, t, line[1:])
		} else {
			lerr = fmt.Errorf("FIXME: unknown command %q", line)
		}
	} else {
		// TODO: handle properly
		lerr = fmt.Errorf("FIXME: zero-word line received")
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
	request.response <- []string{"ACK", acktype, lstr}
	return finished
}

func handleRead(_ chan<- []string, response chan<- []string, t *TrackDB, args []string) (bool, error) {
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
