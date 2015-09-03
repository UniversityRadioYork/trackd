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
	"github.com/UniversityRadioYork/urydb-go"
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

	db, err := urydb.GetDB()
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
				map[string]bifrost.ResourceNoder{
					"state": &StateResourceNode{
						state: "running",
						stateChangeFn: func(x string) (string, error) { return "", fmt.Errorf("cannot change state to %q", x) },
					},
				},
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

type StateResourceNode struct {
	bifrost.ResourceNode

	// TODO(CaptainHayashi): tighten this up?
	state string

	// Called when the state is changed to something other than quitting.
	// Passed the new state verbatim -- please use strings.EqualFold etc. to compare.
	// Return (new state, nil) if the state change is allowed; (_, error) otherwise.
	stateChangeFn func(string) (string, error)
}

func (r *StateResourceNode) NRead(prefix, relpath []string) ([]bifrost.Resource, error) {
	// We don't have any children (though eventually enums will be a thing?)
	if len(relpath) != 0 {
		return []bifrost.Resource{}, fmt.Errorf("state has no children, got %q", relpath)
	}

	return bifrost.ToResource(prefix, r.state), nil
}
func (r *StateResourceNode) NWrite(prefix, relpath []string, val bifrost.BifrostType) error {
	log.Printf("trying to set state to %s", val)

	if len(relpath) != 0 {
		return fmt.Errorf("state has no children, got %q", relpath)
	}

	// TODO(CaptainHayashi): support more than strings here?
	st, ok := val.(bifrost.BifrostTypeString)
	if !ok {
		return fmt.Errorf("state must be a string, got %q", val)
	}
	_, s := st.ResourceBody()

	// Quitting is monotonic: once you've quit, you can't unquit.
	if strings.EqualFold(r.state, "quitting") {
		return fmt.Errorf("cannot change state, server is quitting")
	}

	// Don't allow changes from one state to itself.
	if strings.EqualFold(r.state, s) {
		return nil
	}

	// We handle quitting on our own.
	news := "quitting"
	var err error
	if !strings.EqualFold(s, "quitting") {
		news, err = r.stateChangeFn(s)
		if err != nil {
			return err
		}
	}

	r.state = news
	return nil
}

func (r *StateResourceNode) NDelete(prefix, relpath []string) error {
	// Deleting = writing "quitting" by design.
	// Since we can't write to children of a state node, this is sound.
	return r.NWrite(prefix, relpath, bifrost.BifrostTypeString("quitting"))
}

func (r *StateResourceNode) NAdd(_, _ []string, _ bifrost.ResourceNoder) error {
	// TODO(CaptainHayashi): correct error
	return fmt.Errorf("can't add to state")
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

func handleWrite(_ chan<- *bifrost.Message, response chan<- *bifrost.Message, args []string, it interface{}) (bool, error) {
	t := it.(bifrost.ResourceNoder)

	// write TAG(ignored) PATH VALUE
	if 3 == len(args) {
		// TODO(CaptainHayashi): figuring out if the server has quit is very convoluted at the moment

		res := bifrost.Write(t, args[1], args[2])
		// TODO(CaptainHayashi): don't unpack this (as above)?
		if res.Status.Code != bifrost.StatusOk {
			return false, fmt.Errorf("fixme: %q", res.Status.String())
		}

		// Ugh... please fix this.
		return args[1] == "/control/state" && strings.EqualFold(args[2], "quitting"), nil
	}

	return false, fmt.Errorf("FIXME: bad write %q", args)
}
