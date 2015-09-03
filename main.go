package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/UniversityRadioYork/bifrost-go"
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
	s := NewEnumResourceNode([]string{"running", "quitting"})

	// TODO(CaptainHayashi): factor this out?
	rtree := bifrost.NewDirectoryResourceNode(
		map[string]bifrost.ResourceNoder{
			"control": bifrost.NewDirectoryResourceNode(
				map[string]bifrost.ResourceNoder{"state": s},
			),
			"tracks": &TrackResourceNode{
				trackdb: t,
			},
		},
	)

	log.Printf("listening on %s", *hostport)

	td := tcpserver.New("trackd", *hostport, rtree)
	td.AddHandler(bifrost.RqRead, handleRead).AddHandler(bifrost.RqWrite, handleWrite).Serve()
}

// EnumResourceNode is the type of resource nodes that can hold one of a fixed set of values.
//
// EnumResourceNodes are case-insensitive.
//
// EnumResourceNodes hold three parameters:
// - `current`, which is the current value of the enum;
// - `allowed`, which is the list of allowed values for this enum;
// - `terminal`, which, if set to a value in `allowed`, will be the value to which the enum is set if deleted.
//
// EnumResourceNodes can be accessed using the resource API in the following ways:
//
// Read   /        -> Resource {current: /current, allowed: /allowed}
// Read   /current -> Resource (current value as a string)
// Read   /allowed -> Resource (allowed values as a directory indexed from 0 up)
//
// Write  /        -> As `Write /current`
// Write  /current -> If payload is in /allowed, sets /current to payload.
//                    Otherwise, throw error.
// Write  /allowed -> *not allowed*.
//
// Delete /        -> *not allowed*.
// Delete /current -> *not allowed*.
// Delete /allowed -> *not allowed*.
type EnumResourceNode struct {
	bifrost.ResourceNode

	// These are exported mainly to make NRead able to use ToResource.

	// Current is the current state of the EnumResourceNode.
	Current string `res:"current"`
	// Allowed is the set of allowed states of the EnumResourceNode.
	Allowed []string `res:"allowed"`
}

// NewEnumResourceNode creates a new EnumResourceNode.
//
// The node will have initial value `allowed[0]`.
func NewEnumResourceNode(allowed []string) *EnumResourceNode {
	return &EnumResourceNode{
		Current: allowed[0],
		Allowed: allowed,
	}
}

func isCurrent(relpath []string) bool {
	return len(relpath) == 1 && strings.EqualFold(relpath[0], "current")
}

func (r *EnumResourceNode) isAllowed(state string) bool {
	for _, a := range r.Allowed {
		if strings.EqualFold(state, a) {
			return true
		}
	}
	return false
}

func (r *EnumResourceNode) NRead(prefix, relpath []string) (bifrost.ResourceNoder, error) {
	// Is this looking at the whole node?  If so, just send it,
	if len(relpath) == 0 {
		return r, nil
	}

	// We have two (scalar-ish) children, so maybe this is trying to get one of those.
	// But it'll be easier to knock out the error case first.
	if len(relpath) != 1 {
		return nil, fmt.Errorf("can't find %q", relpath)
	}

	// Which child is it?
	if strings.EqualFold(relpath[0], "current") {
		return bifrost.NewEntryResourceNode(bifrost.BifrostTypeString(r.Current)), nil
	}
	if strings.EqualFold(relpath[0], "allowed") {
		// TODO(CaptainHayashi): automate?
		dir := bifrost.DirectoryResourceNode{}
		for ix, a := range r.Allowed {
			dir[strconv.Itoa(ix)] = bifrost.NewEntryResourceNode(bifrost.BifrostTypeString(a))
		}
		return dir, nil
	}

	return nil, fmt.Errorf("can't find %q", relpath)
}

func (r *EnumResourceNode) NWrite(prefix, relpath []string, val bifrost.BifrostType) error {
	// Trying to write to an enum is the same as trying to write to its current value.
	// Nothing else can be written.
	if !(len(relpath) == 0 || isCurrent(relpath)) {
		return fmt.Errorf("can't write to %q", relpath)
	}

	// TODO(CaptainHayashi): support more than strings here?
	st, ok := val.(bifrost.BifrostTypeString)
	if !ok {
		return fmt.Errorf("state must be a string, got %q", val)
	}
	_, s := st.ResourceBody()

	if !r.isAllowed(s) {
		return fmt.Errorf("%s is not an allowed state", s)
	}

	r.Current = s
	return nil
}

func (r *EnumResourceNode) NDelete(prefix, relpath []string) error {
	// Deleting = writing "quitting" by design.
	// Since we can't write to children of a state node, this is sound.
	return r.NWrite(prefix, relpath, bifrost.BifrostTypeString("quitting"))
}

func (r *EnumResourceNode) NAdd(_, _ []string, _ bifrost.ResourceNoder) error {
	// TODO(CaptainHayashi): correct error
	return fmt.Errorf("can't add to state")
}

type TrackResourceNode struct {
	bifrost.ResourceNode

	trackdb *TrackDB
}

func (r *TrackResourceNode) NRead(prefix, relpath []string) (bifrost.ResourceNoder, error) {
	// Is this about us, or one of our children?
	if len(relpath) == 0 {
		// TODO(CaptainHayashi): This should be something else.
		// Like, maybe, a Query?
		return bifrost.DirectoryResourceNode{}, nil
	}
	// We're expecting relpath to contain the trackID and nothing else.
	// Bail out if this isn't the case.
	if len(relpath) != 1 {
		return nil, fmt.Errorf("expected only one child, got %q", relpath)
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
		for _, r := range bifrost.ToResource(res.Path, res.Node) {
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
