package main

import (
	"database/sql"
	"strconv"

	"github.com/UniversityRadioYork/bifrost-go"
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

// Resolver is a type of function used to resolve track/record IDs.
// It takes a trackID and recordID respectively, and returns the
// resolved file path and/or an error.
type Resolver func(string, string) (string, error)

// TrackDB is a struct containing information on how to consult a URY/MyRadio
// track database.
type TrackDB struct {
	db       *sql.DB
	resolver Resolver
}

// NewTrackDB constructs a new TrackDB from a SQL handle and resolver hook.
func NewTrackDB(db *sql.DB, resolver Resolver) *TrackDB {
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

// LookupTrack looks up a track given its resource name (track ID).
// It returns Bifrost responses describing the track, rooted at prefix.
func (t *TrackDB) LookupTrack(prefix []string, trackres string) (bifrost.ResourceNoder, error) {
	trackid, err := strconv.ParseUint(trackres, 10, 64)
	if err != nil {
		return nil, err
	}

	track, err := t.getTrackInfo(trackid)
	if err != nil {
		return nil, err
	}

	path, err := t.resolver(strconv.Itoa(track.RecordID), trackres)
	if err != nil {
		return nil, err
	}
	track.Path = path

	return bifrost.ToNode(track), nil
}
