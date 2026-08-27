package mewsync

import (
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"github.com/godbus/dbus/v5"
)

const mprisBusPrefix = "org.mpris.MediaPlayer2."
const mprisObjectPath = dbus.ObjectPath("/org/mpris/MediaPlayer2")
const mprisPlayerIface = "org.mpris.MediaPlayer2.Player"

// mprisAvailable reports whether the session bus is reachable and at least
// one MPRIS player is currently registered on it.
func mprisAvailable() bool {
	conn, err := connectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()
	name, err := findMPRISPlayer(conn)
	return err == nil && name != ""
}

func connectSessionBus() (*dbus.Conn, error) {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, err
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// fetchMPRISPlayer reads playback state from the first active MPRIS player
// on the session bus, talking to org.mpris.MediaPlayer2.Player directly
// over D-Bus. A nil state with nil error means no MPRIS player is currently
// running, matching the "nothing playing" convention used elsewhere.
func fetchMPRISPlayer() (*PlayerState, error) {
	conn, err := connectSessionBus()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	name, err := findMPRISPlayer(conn)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, nil
	}
	return readMPRISPlayerState(conn, name)
}

// findMPRISPlayer returns the first bus name starting with
// "org.mpris.MediaPlayer2." found via ListNames, or "" if none is running.
func findMPRISPlayer(conn *dbus.Conn) (string, error) {
	var names []string
	if err := conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return "", err
	}
	for _, n := range names {
		if strings.HasPrefix(n, mprisBusPrefix) {
			return n, nil
		}
	}
	return "", nil
}

func readMPRISPlayerState(conn *dbus.Conn, busName string) (*PlayerState, error) {
	obj := conn.Object(busName, mprisObjectPath)

	statusVar, err := getMPRISProperty(obj, "PlaybackStatus")
	if err != nil {
		return nil, err
	}
	status, _ := statusVar.Value().(string)
	if status == "" {
		return nil, nil
	}

	metadataVar, err := getMPRISProperty(obj, "Metadata")
	if err != nil {
		return nil, err
	}
	metadata, _ := metadataVar.Value().(map[string]dbus.Variant)

	title := mprisMetaString(metadata, "xesam:title")
	if title == "" {
		return nil, nil
	}
	artist := mprisMetaArtist(metadata)

	trackID := mprisMetaString(metadata, "xesam:trackid")
	if trackID == "" {
		trackID = hashTitleArtist(title, artist)
	}

	lengthUs := mprisMetaUint64(metadata, "mpris:length")

	var positionUs uint64
	if positionVar, err := getMPRISProperty(obj, "Position"); err == nil {
		positionUs = variantToUint64(positionVar.Value())
	}

	return &PlayerState{
		TrackID:    trackID,
		Name:       title,
		Artist:     artist,
		IsPlaying:  strings.EqualFold(status, "Playing"),
		ProgressMs: positionUs / 1000,
		DurationMs: lengthUs / 1000,
	}, nil
}

func getMPRISProperty(obj dbus.BusObject, name string) (dbus.Variant, error) {
	var v dbus.Variant
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, mprisPlayerIface, name).Store(&v)
	return v, err
}

func mprisMetaString(m map[string]dbus.Variant, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.Value().(string)
	return s
}

func mprisMetaArtist(m map[string]dbus.Variant) string {
	v, ok := m["xesam:artist"]
	if !ok {
		return ""
	}
	switch t := v.Value().(type) {
	case []string:
		if len(t) > 0 {
			return t[0]
		}
	case string:
		return t
	}
	return ""
}

func mprisMetaUint64(m map[string]dbus.Variant, key string) uint64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	return variantToUint64(v.Value())
}

// variantToUint64 handles the handful of numeric D-Bus types MPRIS servers
// use for Position and mpris:length (they vary between players: some use
// int64, some int32).
func variantToUint64(v any) uint64 {
	switch t := v.(type) {
	case int64:
		if t < 0 {
			return 0
		}
		return uint64(t)
	case uint64:
		return t
	case int32:
		if t < 0 {
			return 0
		}
		return uint64(t)
	case uint32:
		return uint64(t)
	case float64:
		if t < 0 {
			return 0
		}
		return uint64(t)
	default:
		return 0
	}
}

func hashTitleArtist(title, artist string) string {
	h := sha1.Sum([]byte(artist + "\x00" + title))
	return hex.EncodeToString(h[:])
}
