package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/UniversityRadioYork/baps3-go"
	"github.com/UniversityRadioYork/bifrost-server/request"
	"github.com/UniversityRadioYork/bifrost-server/tcpserver"
	//"github.com/docopt/docopt-go"
	_ "github.com/lib/pq"
)

var hostport = flag.String("hostport", "localhost:8123", "The host and port on which trackd should listen (host:port).")
var resolver = flag.String("resolver", "resolve", "The two-argument command to which trackids will be sent on stdin.")

func resolve(recordid, trackid string) (out string, err error) {
	cmd := exec.Command(*resolver, recordid, trackid)

	var outb []byte
	outb, err = cmd.Output()
	out = string(outb)

	return
}

func main() {
	flag.Parse()

	sample, serr := resolve("recordid", "trackid")
	if serr != nil {
		log.Fatal(serr)
	}

	log.Printf("example resolve: %s recordid trackid -> %s", *resolver, sample)

	db, err := getDB()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	t := NewTrackDB(db, resolve)

	log.Printf("listening on %s", *hostport)
	tcpserver.Serve(request.Map{
		baps3.RqRead:  handleRead,
		baps3.RqWrite: handleWrite,
	}, t, "trackd", *hostport)
}

func handleRead(_ chan<- *baps3.Message, response chan<- *baps3.Message, args []string, it interface{}) (bool, error) {
	t := it.(*TrackDB)

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
		return false, fmt.Errorf("FIXME: bad read %q", args)
	}

	return false, nil
}

func handleWrite(_ chan<- *baps3.Message, response chan<- *baps3.Message, args []string, _ interface{}) (bool, error) {
	// write TAG(ignored) PATH VALUE
	if 3 == len(args) {
		resources := strings.Split(strings.Trim(args[1], "/"), "/")
		if len(resources) == 2 && resources[0] == "control" && resources[1] == "state" {
			if strings.EqualFold(args[2], "Quitting") {
				return true, nil
			}
			return false, fmt.Errorf("FIXME: unknown state %q", args[2])
		} else {
			return false, fmt.Errorf("FIXME: unknown write %q", resources)
		}
	} else {
		return false, fmt.Errorf("FIXME: bad write %q", args)
	}

	return false, nil
}
