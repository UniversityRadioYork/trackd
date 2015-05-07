package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"github.com/UniversityRadioYork/baps3-go"
	"github.com/docopt/docopt-go"
	_ "github.com/lib/pq"
	"io"
	"log"
	"os"
	"reflect"
	"strconv"
	"text/tabwriter"
)

var (
	// ErrNoURYDB is the error thrown when URYDB is not present in the environment.
	ErrNoURYDB = errors.New("URYDB not in environment")
	// ErrNoConnFile is the error thrown when there is no urydb connection file.
	ErrNoConnFile = errors.New("couldn't find any connection file")
)

// ConnFiles is the list of possible places to search for a urydb file.
var ConnFiles = []string{
	".urydb",
	"${HOME}/.urydb",
	"/etc/urydb",
	"/usr/local/etc/urydb",
}

func getConnString() (connString string, err error) {
	connString, err = getConnStringEnv()
	if err != nil {
		connString, err = getConnStringFile()
	}
	return
}

func getConnStringEnv() (connString string, err error) {
	connString, err = os.Getenv("URYDB"), nil
	if connString == "" {
		err = ErrNoURYDB
	}
	return
}

func getConnStringFile() (connString string, err error) {
	connString = ""

	for _, rawPath := range ConnFiles {
		path := os.ExpandEnv(rawPath)
		file, ferr := os.Open(path)
		if ferr != nil {
			connString = ""
			continue
		}

		bufrd := bufio.NewReader(file)
		connString, ferr = bufrd.ReadString('\n')

		if ferr != nil {
			connString = ""
			continue
		}

		return
	}

	if connString == "" {
		err = ErrNoConnFile
	}
	return
}

func getWriter(output io.Writer) io.Writer {
	writer := new(tabwriter.Writer)
	writer.Init(os.Stdout, 0, 8, 1, ' ', 0)
	return writer
}

func getDB() (*sql.DB, error) {
	connString, err := getConnString()
	if err != nil {
		return nil, err
	}

	return sql.Open("postgres", connString)
}

// Track is the structure of information for one track.
type Track struct {
	Path         string
	Title        string
	Artist       string
	RecordID     int
	RecordTitle  string
	RecordArtist string
	RecentPlays  uint64
}

// Resource is a structure containing the path, type, and value of a RES response.
type Resource struct {
	path  string
	rtype string
	value string
}

func toResource(url string, item interface{}) []Resource {
	val := reflect.ValueOf(item)
	typ := reflect.TypeOf(item)

	switch val.Kind() {
	case reflect.Struct:
		return structToResource(url, val, typ)
	case reflect.Array, reflect.Slice:
		return sliceToResource(url, val, typ)
	default:
		return []Resource{Resource{url, "entry", fmt.Sprint(item)}}
	}
}

func structToResource(url string, val reflect.Value, typ reflect.Type) []Resource {
	nf := val.NumField()
	af := nf

	// First, announce the incoming directory.
	// We'll fix the value later.
	res := []Resource{Resource{url, "directory", "?"}}

	// Now, recursively work out the fields.
	for i := 0; i < nf; i++ {
		fieldt := typ.Field(i)

		// We can't announce fields that aren't exported.
		// If this one isn't, knock one off the available fields and ignore it.
		if fieldt.PkgPath != "" {
			af--
			continue
		}

		// Work out the resource name from the field name/tag.
		tag := fieldt.Tag.Get("res")
		if tag == "" {
			tag = fieldt.Name
		}

		// Now, recursively emit and collate each resource.
		fieldv := val.Field(i)
		res = append(res, toResource(url+"/"+tag, fieldv.Interface())...)
	}

	// Now fill in the final available fields count
	res[0].value = strconv.Itoa(af)

	return res
}

func sliceToResource(url string, val reflect.Value, typ reflect.Type) []Resource {
	len := val.Len()

	// As before, but now with a list and indexes.
	res := []Resource{Resource{url, "list", strconv.Itoa(len)}}

	for i := 0; i < len; i++ {
		fieldv := val.Index(i)
		res = append(res, toResource(url+"/"+strconv.Itoa(i), fieldv.Interface())...)
	}

	return res
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
    trackd TRACKID

Options:
    -h, --help     Show this message.
    -v, --version  Show version.
`
	arguments, err := docopt.Parse(usage, nil, true, "trackd 0.0", true)
	if err != nil {
		log.Fatal(err)
	}

	trackid, err := strconv.ParseUint(arguments["TRACKID"].(string), 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	db, err := getDB()
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	track, err := getTrackInfo(trackid, db)

	if err != nil {
		log.Fatal(err)
	}

	track.Path = fmt.Sprintf(`M:\%d\%d`, track.RecordID, trackid)

	writer := getWriter(os.Stdout)
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
	fmt.Fprintf(writer, "%s\n", tpack)
}
