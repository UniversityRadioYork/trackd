// SQL queries for trackd.
package main

const (
	// SQLTrackInfo is a SQL query returning general info for one track.
	// It takes one parameter, namely the track ID.
	SQLTrackInfo = `
		SELECT
			rec_track.recordid AS recordid,
			rec_track.title AS ttitle,
			rec_track.artist AS tartist,
			rec_record.title AS rtitle,
			rec_record.artist AS rartist
		FROM
			rec_track
			JOIN rec_record USING(recordid)
		WHERE
			rec_track.trackid = $1
		;
	`
	// SQLTrackRecentPlays is a SQL query returning the number of 'recent' plays for one track.
	// It takes two parameters: the track ID, and a PostgreSQL duration value (eg "3 hours").
	SQLTrackRecentPlays = `
		SELECT
			COUNT(audiologid)
		FROM
			tracklist.track_rec
			JOIN tracklist.tracklist USING(audiologid)
		WHERE
			tracklist.track_rec.trackid = $1
			AND (NOW() - timestart) < $2
		;
	`
)
