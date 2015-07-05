package main

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"

	"github.com/UniversityRadioYork/baps3-go"
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
	db      *sql.DB
	pathFmt string
}

func NewTrackDB(db *sql.DB, pathFmt string) *TrackDB {
	tdb := new(TrackDB)
	tdb.db = db
	tdb.pathFmt = pathFmt

	return tdb
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

	track.Path = fmt.Sprintf(t.pathFmt, track.RecordID, trackid)

	urlstub := fmt.Sprintf("/tracks/%d", trackid)
	res := toResource("", track)
	for _, r := range res {
		emitRes(output, urlstub, r.rtype, r.path, r.value)
	}
}
