# trackd
Track resolver for URY's BAPS3 system.

## Usage

### Starting

Start `trackd` like this:

`trackd --hostport="host:port" --resolver="resolve_program"`

where `resolve_program` is a program that accepts two unnamed arguments—the
record and track identifiers—and prints onto stdout the path to the
thus-identified program.  A simple shell-script to do so might be:

```
#!/bin/sh
printf '/home/music/%s/%s' "${1}" "${2}"
```

### Database configuration

`trackd` looks for a file containing a PostgreSQL connection string of the form
`postgres://username:password@host/database?options` in `.urydb`, `~/.urydb`,
`/etc/urydb` or `/usr/local/etc/urydb` (in that order).  This is probably going
to be a temporary arrangement.  **Please remember to make sure that only the
trackd user can read this file.**

### Basic Usage

`trackd` listens on the given port, binding to the given host, using the
Bifrost protocol.  You can ask it for track information by `nc`ing onto said
host and port and sending

`read tag /tracks/trackid`

where `trackid` is the ID of the track you wish to resolve in the URY (or
MyRadio-compatible) track database, and `tag` is some arbitrary identifier
used to identify which `ACK` (return status) corresponds to the `read`
command later.

The result of a successful query (for, say, track ID `456`) looks like:

```
RES /tracks/456 directory 7
RES /tracks/456/path entry 'M:\123\456'
RES /tracks/456/title entry 'Brown Girl In The Ring'
RES /tracks/456/artist entry 'Boney M.'
RES /tracks/456/record_id entry 123
RES /tracks/456/record_title entry 'Nightflight to Venus'
RES /tracks/456/record_artist entry 'Boney M.'
RES /tracks/456/recent_plays entry 0
ACK OK Success read tag /tracks/456
```

