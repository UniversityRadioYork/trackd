package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/UniversityRadioYork/bifrost-go"
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

	// TODO(CaptainHayashi): factor this out?
	rtree := bifrost.NewDirectoryResourceNode(
		map[string]bifrost.ResourceNoder{
			"control": bifrost.NewDirectoryResourceNode(
				// TODO: put state here
				map[string]bifrost.ResourceNoder{},
			),
			"tracks": &TrackResourceNode{
				trackdb: t,
			},
		},
	)

	log.Printf("listening on %s", *hostport)
	tcpserver.Serve(request.Map{
		bifrost.RqRead:  handleRead,
		bifrost.RqWrite: handleWrite,
	}, rtree, "trackd", *hostport)
}

type TrackResourceNode struct {
	bifrost.ResourceNode

	trackdb *TrackDB
}

func (r *TrackResourceNode) NRead(prefix, relpath []string) ([]bifrost.Resource, error) {
	// Is this about us, or one of our children?
	if len(relpath) == 0 {
		// TODO(CaptainHayashi): This should be something else.
		// Like, maybe, a Query?
		return bifrost.ToResource(prefix, struct{}{}), nil
	}
	// We're expecting relpath to contain the trackID and nothing else.
	// Bail out if this isn't the case.
	if len(relpath) != 1 {
		return []bifrost.Resource{}, fmt.Errorf("expected only one child, got %q", relpath)
	}
	return r.trackdb.LookupTrack(prefix, relpath[0])
}

func (r *TrackResourceNode) NWrite(_, _ []string, _ bifrost.BifrostType) error {
	// TODO(CaptainHayashi): correct error
	return fmt.Errorf("can't write to trackdb")
}

func (r *TrackResourceNode) NDelete(_, _ []string) error {
	// TODO(CaptainHayashi): correct error
	return fmt.Errorf("can't delete trackdb")
}

func (r *TrackResourceNode) NAdd(_, _ []string, _ bifrost.ResourceNoder) error {
	// TODO(CaptainHayashi): correct error
	return fmt.Errorf("can't add to trackdb")
}

func handleRead(_ chan<- *bifrost.Message, response chan<- *bifrost.Message, args []string, it interface{}) (bool, error) {
	t := it.(bifrost.ResourceNoder)

	// read TAG PATH
	if 2 == len(args) {
		// Reading can never quit the server (we hope).
		res := bifrost.Read(t, args[1])
		// TODO(CaptainHayashi): don't unpack this?
		if res.Status.Code != bifrost.StatusOk {
			return false, fmt.Errorf("fixme: %q", res.Status.String())
		}
		for _, r := range res.Resources {
			response <- r.Message(args[0])
		}

		return false, nil
	}

	return false, fmt.Errorf("FIXME: bad read %q", args)
}

func handleWrite(_ chan<- *bifrost.Message, response chan<- *bifrost.Message, args []string, _ interface{}) (bool, error) {
	// write TAG(ignored) PATH VALUE
	if 3 == len(args) {
		resources := strings.Split(strings.Trim(args[1], "/"), "/")
		if len(resources) == 2 && resources[0] == "control" && resources[1] == "state" {
			if strings.EqualFold(args[2], "Quitting") {
				return true, nil
			}
			return false, fmt.Errorf("FIXME: unknown state %q", args[2])
		}
		return false, fmt.Errorf("FIXME: unknown write %q", resources)
	}

	return false, fmt.Errorf("FIXME: bad write %q", args)
}
