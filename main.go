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

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConnection(conn, db)
	}
}

func handleConnection(conn net.Conn, db *sql.DB) {
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
			// TODO: handle quit
			// TODO: handle bad command
			if 0 < len(line) {
				// TODO: split up this almighty switch statement
				switch line[0] {
				case "read":
					handleRead(conn, db, line[1:])
				default:
					// TODO: write
					// TODO: delete
					log.Printf("FIXME: unknown command %q", line)
				}
			} else {
				// TODO: handle properly
				log.Printf("FIXME: zero-word line received")
			}
		}
	}
}

func handleRead(conn net.Conn, db *sql.DB, args []string) {
	// TODO: handle trailing slash
	if 1 == len(args) {
		resources := strings.Split(strings.Trim(args[0], "/"), "/")
		if len(resources) == 2 && resources[0] == "tracks" {
			log.Printf("LOOKUP %q", resources[1])
			lookupTrack(conn, db, resources[1])
		} else {
			log.Printf("FIXME: unknown read %q", resources)
		}
	} else {
		// TODO: send failure here
		log.Printf("FIXME: bad read %q", args)
	}
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
