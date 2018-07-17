package status

import (
	"encoding/json"
	"fmt"
	"os"
	p "path"
	fp "path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	stateFileName = "watch"

	// The duration over which work events are consolidated (all events that
	// happen within a 'eventBucketSize'-length period of time are registered as a
	// single event)
	eventBucketSize = 3 * time.Second
)

// Watch is an object that watches directories for changes that happen below
// them, by watching all subdirectories, and adding new watches when new child
// directories are created
type Watch struct {
	// The directory where tg is storing its state
	tgStateDir string

	// stateFile is an open file descriptor for the file in tgStateDir where this
	// Watch stores and retrieves its state
	stateFile *os.File

	// inotifyFd is the unix file descriptor where inotify events corresponding
	// to writes in the watched directories can be read
	inotifyFd int

	// watches map paths to Toggl projects. When a write occurs under any key
	// a time entry will be created/extended in the corresponding project
	rootWatches map[string]string

	// wdToPath maps watch descriptors to directories being watched, so that
	// watch events can be matched to a directory
	wdToPath map[int]string

	// callbackMu protects 'callback'
	callbackMu sync.Mutex

	// callback is called whenever a file event is observed
	callback func()
}

// MarshalJSON satisfies the json.Marshaller interface
func (w *Watch) MarshalJSON() ([]byte, error) {
	return json.Marshal(w.rootWatches)
}

// UnmarshalJSON satisfies the json.Unmarshaller interface
func (w *Watch) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &w.rootWatches)
}

// lock acquires an advisory lock on the file opened at fd. For more on
// advisory locking, see https://gavv.github.io/blog/file-locks/
func lock(fd int) error {
	for {
		// exclusive lock, nonblocking call (fail fast)
		err := unix.Flock(fd, unix.LOCK_NB|unix.LOCK_EX)
		if err == nil {
			return nil
		}
		switch err {
		case unix.EINTR:
			continue // interrupted--retry syscall
		case unix.EWOULDBLOCK:
			return fmt.Errorf("another watch process is already running")
		default:
			return fmt.Errorf("error locking watch file: %v", err)
		}
	}
}

func (w *Watch) addWatch(path string) error {
	// Walk the directory tree under 'path'
	err := fp.Walk(path, func(path string, info os.FileInfo, err error) error {
		// Only watch directories
		if !info.IsDir() {
			return nil
		}

		// heuristic: skip hidden directories
		// TODO make this flag-controlled
		filename := p.Base(path)
		if strings.HasPrefix(filename, ".") {
			return fp.SkipDir
		}

		// heuristic: avoid golang vendor directories, since I typically use this
		// with go projects
		if filename == "vendor" {
			if _, err := os.Stat(p.Join(p.Dir(path), "Gopkg.lock")); err == nil {
				return fp.SkipDir // vendor dir managed by 'dep'
			}
			if _, err := os.Stat(p.Join(path, "vendor.json")); err == nil {
				return fp.SkipDir // vendor dir managed by 'govendor'
			}
		}

		// Add inotify watch to this child
		wd, err := unix.InotifyAddWatch(w.inotifyFd, path,
			unix.IN_CREATE|unix.IN_DELETE|unix.IN_MODIFY|
				unix.IN_MOVED_FROM|unix.IN_MOVED_TO|
				unix.IN_DELETE_SELF|unix.IN_DELETE_SELF)
		if err != nil {
			return fmt.Errorf("could not add watch: %v", err)
		}
		w.wdToPath[wd] = path
		return nil
	})
	return err
}

// readEvents is a helper function that reads unix inotify events from
// w.inotifyFd and writes empty structs to eventChan. It also installs new
// listeners for new child directories that the user creates
func (w *Watch) readEvents(eventChan chan<- struct{}) {
	buf := make([]byte, 1024*unix.SizeofInotifyEvent) // huge buffer, to hold all events
	for {
		n, err := unix.Read(w.inotifyFd, buf)
		// TODO all of these os.Exit() calls are silly -- try to recover
		// TODO do I need all of these cases?
		switch {
		case n < 0:
			fmt.Fprintf(os.Stderr, "inotify read error: %v", err)
		case n == 0:
			return
		case n < unix.SizeofInotifyEvent:
			fmt.Fprintf(os.Stderr, "short read of %d bytes: %v", n, err)
		case err != nil:
			fmt.Fprintf(os.Stderr, "inotify read error (n != 0?): %v", err)
		default:
			// success
		}
		idx := 0
		for idx < n {
			event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[idx]))
			idx += unix.SizeofInotifyEvent

			// extract name from stat struct
			var name string
			for r := int(event.Len); r > 0; r-- {
				if buf[idx+r-1] != 0 {
					name = string(buf[idx : idx+r])
					break
				}
			}
			idx += int(event.Len)
			path := p.Clean(p.Join(w.wdToPath[int(event.Wd)], name))

			// If event involves creating or moving a subdirectory, add watches for
			// the new subdirectory
			fmt.Printf("event: %s\n", Render(event, path))
			if event.Mask&(unix.IN_CREATE|unix.IN_MOVED_TO) > 0 {
				fInfo, err := os.Stat(path)
				if err != nil {
					// TODO log somewhere real
					fmt.Fprintf(os.Stderr, "could not stat new path %q: %v", path, err)
				}
				if fInfo.IsDir() {
					w.addWatch(path) // Add inotify watch to this child
				}
			}

			// If the event concerns a watch descriptor, update the relevant maps
			if event.Mask&(unix.IN_MOVE_SELF|unix.IN_DELETE_SELF) > 0 {
				// unix.InotifyRmWatch(w.inotifyFd, uint32(event.Wd))
				fmt.Printf("removing %d from %v\n", event.Wd, w.wdToPath)
				delete(w.wdToPath, int(event.Wd))
				fmt.Printf("removing %s from %v\n", path, w.rootWatches)
				delete(w.rootWatches, path)
			}
			eventChan <- struct{}{} // notify watcher that an event has occurred
		}
	}
}

func (w *Watch) handleEvents(eventChan <-chan struct{}) {
	for {
		<-eventChan // wait for an event
		// read as many events as possible in 'eventBucketSize'
		timer := time.After(eventBucketSize)
	waitForEvents:
		for {
			select {
			case <-eventChan:
				continue // discard event
			case <-timer:
				break waitForEvents
			}
		}
		// call callback (but don't hold mutex while callback is running
		// TODO is that really necessary?
		w.callbackMu.Lock()
		cb := w.callback
		w.callbackMu.Unlock()
		if cb != nil {
			cb()
		}
	}
}

// SetCallback sets that function that 'w' calls on write events
func (w *Watch) SetCallback(f func()) {
	w.callbackMu.Lock()
	defer w.callbackMu.Unlock()
	w.callback = f
}

// AddWatch tells this Watch to start monitoring a new directory
func (w *Watch) AddWatch(dir, project string) error {
	_, alreadyWatched := w.rootWatches[dir]
	changedProject := alreadyWatched && w.rootWatches[dir] != project
	if !alreadyWatched || changedProject {
		w.rootWatches[dir] = project
		w.stateFile.Seek(0 /* relative to origin of file */, 0)
		w.stateFile.Truncate(0)
		if err := json.NewEncoder(w.stateFile).Encode(w); err != nil {
			return err
		}
	}
	if !alreadyWatched {
		if err := w.addWatch(dir); err != nil {
			return err
		}
	}
	return nil
}

// Start starts a new watcher, with which child paths can be registered
func Start(tgStateDir string) (*Watch, error) {
	statePath := p.Join(tgStateDir, stateFileName)
	var (
		stateFile *os.File
		err       error
	)
	if _, err = os.Stat(statePath); err != nil {
		stateFile, err = os.OpenFile(statePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0644)
		if err != nil {
			return nil, fmt.Errorf("could not create watch state file: %v", err)
		}
	} else {
		stateFile, err = os.OpenFile(statePath, os.O_RDWR, 0644)
	}
	// lock the state file, to make sure no other process is watching these paths
	if err := lock(int(stateFile.Fd())); err != nil {
		return nil, err
	}

	// Deserialize the list of watched directories from the watch file
	w := &Watch{
		tgStateDir:  tgStateDir,
		rootWatches: make(map[string]string),

		// todo does this need to be in w at all?
		stateFile: stateFile,
		wdToPath:  make(map[int]string),
	}
	if w.stateFile == nil {
		return nil, fmt.Errorf("watchFd is not a valid file descriptor")
	}
	json.NewDecoder(w.stateFile).Decode(w)

	// Create inotify fd and start goroutines to publish and process watch events
	// TODO use an errgroup and context to re-establish watches if w.readEvents
	// fails
	eventChan := make(chan struct{}, 100)
	w.inotifyFd, err = unix.InotifyInit()
	if err != nil {
		return nil, err
	}
	// copy inotify events on w.fd to 'eventChan'
	go w.readEvents(eventChan)
	// Receive/batch events from 'eventChan' and call w.callback() when they occur
	go w.handleEvents(eventChan)

	// Start watching the watched directories
	for path, project := range w.rootWatches {
		if err := w.AddWatch(path, project); err != nil {
			return nil, err // right? Can I handle this error in any meaningful way
		}
	}
	return w, nil
}
