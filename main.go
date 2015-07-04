package main

import (
	"database/sql"
	"fmt"
	"github.com/UniversityRadioYork/baps3-go"
	"github.com/docopt/docopt-go"
	_ "github.com/lib/pq"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
)

// Track is the structure of information for one track.
type Track struct {
	Path         string `res:"path"`
	Title        string `res:"title"`
	Artist       string `res:"artist"`
	RecordID     int    `res:"record_id"`
	RecordTitle  string `res:"record_title"`
	RecordArtist string `res:"record_artist"`
	RecentPlays  uint64 `res:"recent_plays"`
}

func getTrackInfo(trackid uint64, db *sql.DB) (track Track, err error) {
	rows, err := db.Query(SQLTrackInfo, trackid)
	if err != nil {
		return
	}

	for rows.Next() {
		err = rows.Scan(&track.RecordID, &track.Title, &track.Artist, &track.RecordTitle, &track.RecordArtist)
		if err != nil {
			return
		}
	}

	err = rows.Err()
	if err != nil {
		return
	}

	track.RecentPlays, err = getTrackRecentPlays(trackid, db)

	return
}

func getTrackRecentPlays(trackid uint64, db *sql.DB) (plays uint64, err error) {
	rows, err := db.Query(SQLTrackRecentPlays, trackid, "3 hours")
	if err != nil {
		return
	}

	rows.Next()
	err = rows.Scan(&plays)
	return
}

type Request struct {
	contents []string
	response chan<- []string
	ack      chan<- []string
}

type ClientHandle struct {
	// Channel for sending broadcast messages to this client.
	Broadcast chan<- []string

	// Channel for sending disconnection requests to this client.
	Disconnect chan<- bool
}

func main() {
	usage := `FIX

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

	db, err := getDB()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	var wg sync.WaitGroup

	clientPool, _ := NewClientPool()
	go clientPool.Run(&wg)

	requests := make(chan *Request)
	broadcast := make(chan []string)
	go RequestLoop(ln, requests, broadcast, &wg)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConnection(conn, db, &wg)
	}

	wg.Wait()
}

func RequestLoop(ln io.Closer, requests <-chan *Request, broadcast chan<- []string, wg *sync.WaitGroup) {
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}

	for {
		select {
		case <-requests:
			// do something
		}
	}
}

func handleConnection(conn net.Conn, db *sql.DB, wg *sync.WaitGroup) {
	// Make sure the main server waits for this connection to close
	// properly
	if wg != nil {
		wg.Add(1)
		defer wg.Done()
	}

	buf := make([]byte, 1024)
	tok := baps3.NewTokeniser()

	for {
		nbytes, err := conn.Read(buf)
		if err != nil {
			// TODO: handle error correctly, send error to client
			log.Printf("connection read error: %q", err)
			return
		}

		lines, _, err := tok.Tokenise(buf[:nbytes])
		if err != nil {
			// TODO: handle error correctly, retry tokenising perhaps
			log.Printf("connection tokenise error: %q", err)
			return
		}

		for _, line := range lines {
			if finished := handleCommand(conn, db, line); finished {
				return
			}
		}
	}
}

type CmdTable map[string]func(net.Conn, *sql.DB, []string) (bool, error)

var cmds CmdTable = CmdTable{
	"read": handleRead,
	"quit": handleQuit,
}

func handleQuit(_ net.Conn, _ *sql.DB, _ []string) (bool, error) {
	return true, nil
}

func handleCommand(conn net.Conn, db *sql.DB, line []string) bool {
	var lerr error
	finished := false

	// TODO: handle quit
	// TODO: handle bad command
	if 0 < len(line) {
		cmdfunc, ok := cmds[line[0]]
		if ok {
			finished, lerr = cmdfunc(conn, db, line[1:])
		} else {
			lerr = fmt.Errorf("FIXME: unknown command %q", line)
		}
	} else {
		// TODO: handle properly
		lerr = fmt.Errorf("FIXME: zero-word line received")
	}

	ack := "???"
	lstr := "Success"
	if lerr == nil {
		ack = "OK"
	} else {
		// TODO: proper error distinguishment
		ack = "FAIL"
		lstr = lerr.Error()
	}

	oerr := outputAck(conn, ack, lstr, line)
	if oerr != nil {
		log.Println(oerr)
	}

	return finished
}

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

func handleRead(conn net.Conn, db *sql.DB, args []string) (bool, error) {
	// read TAG(ignored) PATH
	if 2 == len(args) {
		resources := strings.Split(strings.Trim(args[1], "/"), "/")
		if len(resources) == 2 && resources[0] == "tracks" {
			log.Printf("LOOKUP %q", resources[1])
			lookupTrack(conn, db, resources[1])
		} else {
			return false, fmt.Errorf("FIXME: unknown read %q", resources)
		}
	} else {
		// TODO: send failure here
		return false, fmt.Errorf("FIXME: bad read %q", args)
	}

	return false, nil
}

func lookupTrack(writer io.Writer, db *sql.DB, trackres string) {
	trackid, err := strconv.ParseUint(trackres, 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	track, err := getTrackInfo(trackid, db)
	if err != nil {
		log.Fatal(err)
	}

	track.Path = fmt.Sprintf(`M:\%d\%d`, track.RecordID, trackid)

	urlstub := fmt.Sprintf("/tracks/%d", trackid)
	res := toResource("", track)
	for _, r := range res {
		emitRes(writer, urlstub, r.rtype, r.path, r.value)
	}
}

func emitRes(writer io.Writer, urlstub string, restype string, resname string, resval string) {
	tmsg := baps3.NewMessage(baps3.RsRes).AddArg(urlstub + resname).AddArg(restype).AddArg(resval)
	tpack, err := tmsg.Pack()
	if err != nil {
		log.Fatal(err)
	}
	writer.Write(tpack)
}
