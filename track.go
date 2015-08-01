package main

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"

	"github.com/UniversityRadioYork/baps3-go"
	bsrv "github.com/UniversityRadioYork/bifrost-server"
	_ "github.com/lib/pq"
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

type TrackDB struct {
	db       *sql.DB
	resolver func(string, string) (string, error)
}

func NewTrackDB(db *sql.DB, resolver func(string, string) (string, error)) *TrackDB {
	return &TrackDB{db: db, resolver: resolver}
}

func (t *TrackDB) getTrackInfo(trackid uint64) (track Track, err error) {
	rows, err := t.db.Query(SQLTrackInfo, trackid)
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

	track.RecentPlays, err = t.getTrackRecentPlays(trackid)

	return
}

func (t *TrackDB) getTrackRecentPlays(trackid uint64) (plays uint64, err error) {
	rows, err := t.db.Query(SQLTrackRecentPlays, trackid, "3 hours")
	if err != nil {
		return
	}

	rows.Next()
	err = rows.Scan(&plays)
	return
}

func (t *TrackDB) LookupTrack(output chan<- *baps3.Message, trackres string) {
	trackid, err := strconv.ParseUint(trackres, 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	track, err := t.getTrackInfo(trackid)
	if err != nil {
		log.Fatal(err)
	}

	path, err := t.resolver(strconv.Itoa(track.RecordID), trackres)
	if err != nil {
		log.Fatal(err)
	}
	track.Path = path

	urlstub := fmt.Sprintf("/tracks/%d", trackid)
	res := bsrv.ToResource("", track)
	for _, r := range res {
		emitRes(output, urlstub, r.Type, r.Path, r.Value)
	}
}

func emitRes(output chan<- *baps3.Message, urlstub string, restype string, resname string, resval string) {
	output <- baps3.NewMessage(baps3.RsRes).AddArg(urlstub + resname).AddArg(restype).AddArg(resval)
}
