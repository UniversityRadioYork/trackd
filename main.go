package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/UniversityRadioYork/baps3-go"
	"github.com/UniversityRadioYork/bifrost-server/request"
	"github.com/UniversityRadioYork/bifrost-server/tcpserver"
	"github.com/docopt/docopt-go"
	_ "github.com/lib/pq"
)

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

	tcpserver.Serve(request.Map{
		baps3.RqRead: func(b, r chan<- *baps3.Message, s []string) (bool, error) { return handleRead(b, r, t, s) },
		baps3.RqQuit: func(_, _ chan<- *baps3.Message, _ []string) (bool, error) { return true, nil },
	}, "trackd", arguments["HOSTPORT"].(string))
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
