package status

import (
	"fmt"
	"os"
	p "path"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func j(paths ...string) string {
	return p.Join(paths...)
}

type (
	// AtLeast (in CheckEvent(t, AtLeast(5), events) tells CheckEvent to expect
	// at least 5 structs from 'events'
	AtLeast int
	// AtMost (in CheckEvent(t, AtMost(5), events) tells CheckEvent to expect
	// at most 5 structs from 'events'
	AtMost int
	// Exactly (in CheckEvent(t, Exactly(5), events) tells CheckEvent to expect
	// exactly 5 structs from 'events'
	Exactly int
)

// CheckEvent checks that an appropriate quantity of structs have been written
// to 'events' (it's assumed that a watcher publishes a struct to 'events'
// every time a new inotify event is received
func CheckEvent(t testing.TB, count interface{}, events chan struct{}) {
	t.Helper()
	eventCount := 0

	// Wait at least eventBucketSize+1s for the first event(s) to register, and
	// keep reading events until no more events are coming
waitForEvents:
	for {
		select {
		case _, ok := <-events:
			if !ok {
				break // channel closed
			}
			eventCount++
		case <-time.After(eventBucketSize * 2 /*+ time.Second*/):
			break waitForEvents // no work events in last 'eventBucketSize'
		}
	}

	// Make sure we met the count condition
	switch v := count.(type) {
	case AtLeast:
		if eventCount < int(v) {
			t.Fatalf("expected at least %d events, but only saw %d", v, eventCount)
		}
	case AtMost:
		if eventCount > int(v) {
			t.Fatalf("expected at most %d events, but only saw %d", v, eventCount)
		}
	case Exactly:
		if eventCount != int(v) {
			t.Fatalf("expected exactly %d events, but only saw %d", v, eventCount)
		}
	default:
		t.Fatal("Unexpected type %T passed to CheckEvent", v)
	}
}

// Render converts unix.InofityEvents to human-readable strings for debugging
func Render(e *unix.InotifyEvent, path string) string {
	var eType string
	if e.Mask&unix.IN_CREATE > 0 {
		eType += "Create/"
	}
	if e.Mask&unix.IN_DELETE > 0 {
		eType += "Delete/"
	}
	if e.Mask&unix.IN_MODIFY > 0 {
		eType += "Modify/"
	}
	if e.Mask&unix.IN_MOVED_FROM > 0 {
		eType += "Move from/"
	}
	if e.Mask&unix.IN_MOVED_TO > 0 {
		eType += "Move to/"
	}
	if e.Mask&unix.IN_DELETE_SELF > 0 {
		eType += "Delete watched dir/"
	}
	if e.Mask&unix.IN_MOVE_SELF > 0 {
		eType += "Move watched dir/"
	}
	if e.Mask&unix.IN_IGNORED > 0 {
		eType += "Ignored/"
	}
	if eType == "" {
		eType = fmt.Sprintf("%x", e.Mask)
	} else {
		eType = eType[:len(eType)-1]
	}
	result := fmt.Sprintf("%s %q", eType, path)

	if e.Mask&(unix.IN_CREATE|unix.IN_MODIFY) > 0 {
		var fInfo os.FileInfo
		fInfo, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not stat %s: %v\n", path, err)
		}
		if fInfo.IsDir() {
			result += " (dir)"
		} else {
			result += " (file)"
		}
	}
	return result
}
