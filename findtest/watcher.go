package main

import (
	"fmt"
	"io"
	"os"
	p "path"
	"sync"

	"golang.org/x/sys/unix"
	// "github.com/glycerine/rbuf"
)

const watchMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MODIFY |
	unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_IGNORED

// inotifyBufSz is the minimum buffer size required to read a unix inotify event
// Per 'man 7 inotify':
//  Specifying a buffer of size 'sizeof(struct inotify_event) + NAME_MAX + 1'
//  will be sufficient to read at least one event.
const inotifyBufSz = unix.SizeofInotifyEvent + unix.NAME_MAX + 1

// Watcher watches a directory
type Watcher struct {
	watchFd int

	// mapMu guards both 'wdToPath' and 'pathToWD'--both are used by Watch.add,
	// which may be called by multiple goroutines
	mapMu sync.RWMutex
	// wdToPath maps watch descriptors to path (used to interpret new inotify
	// events generated by the linux kernel)
	wdToPath map[int]string
	// watchedNow tracks the set of directories currently being watched
	watchedNow map[string]struct{}
	// settingUp tracks the set of directories such that add(dir) has been
	// called, but hasn't yet finished
	settingUp map[string]struct{}
}

// WatchEventType is the type associated with each WatchEvent returned by a
// Watcher
type WatchEventType uint8

// The definition of each of the WatchEventTypes
const (
	Create WatchEventType = iota
	Delete
	Modify
)

// A WatchEvent is passed to a callback by a Watcher to indicate some
// filesystem event
type WatchEvent struct {
	Type WatchEventType
	Path string
}

// NewWatcher constructs a new Watcher struct
func NewWatcher(dir string, cb func(e WatchEvent)) (w *Watcher, err error) {
	// Init inotify file descriptor
	fd, err := unix.InotifyInit()
	if err != nil {
		return nil, err
	}
	w = &Watcher{
		watchFd:     unixFD(fd),
		wdToPath:    make(map[int]string),
		watchedNow:  make(map[string]struct{}),
		watchedSoon: make(map[string][2][]WatchEvent),
	}
	go w.watchInotifyFd()
	add(dir)
}

// watchInodifyFd reads w's watchFd in a loop, and responds to events (by adding
// new subdirs to 'w' and calling the watch callback)
func (w *Watcher) watchInotifyFd() {
	var start, end int
	buf := make([]byte, inotifyBufSz*10)
	for {
		// Read events into 'buf' from w.watchFd
		n, err := unix.Read(w.watchFd, buf[end:])
		// TODO do I need all of these cases? Could err != nil cover some?
		switch {
		case n < 0:
			fmt.Fprintf(os.Stderr, "inotify read error: %v", err)
			os.Exit(1)
		case n == 0:
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "non-EOF error returned from 0-length read: %v", err)
			}
			fmt.Println("EOF")
			return
		case n < unix.SizeofInotifyEvent:
			fmt.Fprintf(os.Stderr, "short read (%d bytes, but len(inotify event) = %d bytes); err: %v",
				n, unix.SizeofInotifyEvent, err)
			os.Exit(1)
		case err != nil:
			fmt.Fprintf(os.Stderr, "inotify read error (n != 0?): %v", err)
			os.Exit(1)
		}
		end += n

		// Consume events from buf, if possible
		for start < end {
			// check if entire event struct was read by previous call
			if start+unix.SizeofInotifyEvent > end {
				break
			}
			// extract inotify event struct (minus name)
			event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[start]))
			// Check if all of 'name' was read by previous call
			if start+event.Len > end {
				break
			}
			// Success--advance 'start'
			start += unix.SizeofInotifyEvent + int(event.Len)

			// extract name
			// Per man 7 inotify:
			//  The name field is present only when an event is returned for a file
			//  inside a watched directory; it identifies the filename within to the
			//  watched directory. This filename is null-terminated, and may include
			//  further null bytes ('\0') to align subsequent reads to a suitable
			//  address boundary.
			var name string
			for r := int(event.Len); r > 0; r-- {
				if buf[start+unix.SizeofInotifyEvent+r-1] != 0 {
					name = string(buf[nextEventStart : nextEventStart+r])
					break
				}
			}
			we := w.toWatchEvent(event, name)

			// Handle 'we' correctly
			w.mapMu.RLock()
			_, pathIsWatched := w.watchedNow[we.Path]
			_, parentIsWatched := w.watchedNow[p.Dir(we.Path)]
			_, pathIsSettingUp := w.watchedNow[we.Path]
			_, parentIsSettingUp := w.watchedNow[p.Dir(we.Path)]
			w.mapMu.RUnlock()
			// determine if 'path' is a dir
			var isDir bool
			if pathIsWatched {
				isDir = true
			} else if we.Type != Delete {
				var fInfo os.FileInfo
				fInfo, err := os.Stat(path)
				isDir = fInfo.IsDir()
			}
			if isDir {
				if we.Type == Create {
					add(we.Path)
				} else if we.Type == Delete {
					w.mapMu.Lock()
					delete(w.watchedNow[we.Path])
					delete(w.watchedNow[we.Path])
					w.mapMu.Unlock()
				}
			}
			/// ........
			switch {
			case pathIsWatched:
				switch we.Type {
				case Create:
				// add(path)
				// cb(message)
				case Delete:
				// remove from map (linux removes inotify, I believe)
				// cb(message)
				case Modify:
					// add(path)
					// cb(message)
				}
			case parentIsWatched:
				// This should always be true...
				// cb(message)
			case we.Type == Delete && pathIsSettingUp:
				// remove from map (linux removes inotify, I believe)
				// cb(message)
			case pathIsSettingUp:
				// cb(message)
				// (how would this happen--we.Type can't be Create if we're already
				// setting it up, and I'm not sure what triggers a Modify event for a
				// dir...)
			case parentIsSettingUp:
				// just publish
			default:
				// wat--how did I get this event? race...
			}
		}

		// Reset 'buf'--move data to the front, so that future data can be appended
		copy(buf, buf[start:end])
		start, end = 0, end-start
	}
}

func (w *Watcher) toWatchEvent(e *unix.InotifyEvent, name string) WatchEvent {
	var result WatchEvent
	switch {
	case e.Mask&unix.IN_CREATE > 0, e.Mask&unix.IN_MOVED_TO > 0:
		result.Type = Create
	case e.Mask&unix.IN_DELETE > 0, e.Mask&unix.IN_MOVED_FROM > 0:
		result.Type = Delete
	case e.Mask&unix.IN_MODIFY > 0:
		result.Type = Modify
	}
	result.Path = p.Clean(p.Join(wdToPath[int(e.Wd)], name))
	return result
}

// add adds a subdirectory of w's original dir to the underlying linux inotify
// instance.
//
// Per 'man 7 watch':
//  If monitoring an entire directory subtree, and a new subdirectory is created
//  in that tree or an existing directory is renamed into that tree, be aware
//  that by the time you create a watch for the new subdirectory, new files (and
//  subdirectories) may already exist inside the subdirectory. Therefore, you
//  may want to scan the contents of the subdirectory immediately after adding
//  the watch (and, if desired, recursively add watches for any subdirectories
//  that it contains).
func (w *Watcher) add(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("watch.Add() could not stat %s: %v", dir, err)
	}

	// Create watch and note the path associated with this watch descriptor
	wd, err := unix.InotifyAddWatch(fd, dir, watchMask)
	if err != nil {
		return fmt.Errorf("could not add watch: %v", err)
	}
	wdToPath[wd] = path

	// Scan the current contents of 'dir' and add any existing subdirs to 'w'
}

// Render returns a human-readable string corresponding to 'e'
func Render(e *unix.InotifyEvent, name string) string {
	var eType string
	switch {
	case e.Mask&unix.IN_CREATE > 0:
		eType = "Create"
	case e.Mask&unix.IN_DELETE > 0:
		eType = "Delete"
	case e.Mask&unix.IN_MODIFY > 0:
		eType = "Modify"
	case e.Mask&unix.IN_MOVED_FROM > 0:
		eType = "Move from"
	case e.Mask&unix.IN_MOVED_TO > 0:
		eType = "Move to"
	case e.Mask&unix.IN_DELETE_SELF > 0:
		eType = "Delete watched dir"
	case e.Mask&unix.IN_MOVE_SELF > 0:
		eType = "Move watched dir"
	}

	path := p.Clean(p.Join(wdToPath[int(e.Wd)], name))
	result := fmt.Sprintf("Event type: %s %s", eType, path)
	if eType == "Create" || eType == "Modify" {
		var fInfo os.FileInfo
		fInfo, err := os.Stat(path)
		if err != nil {
			fmt.Printf("could not stat %s: %#v (%v)\n", path, err, err)
		}
		if fInfo.IsDir() {
			result += " (dir)\n"
		} else {
			result += " (file)\n"
		}
	} else {
		result += "\n"
	}
	return result
}
