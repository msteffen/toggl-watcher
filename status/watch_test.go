package status

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	p "path"
	"runtime"
	"strings"
	"testing"

	// Imported for pprof
	"log"
	"net/http"
	_ "net/http/pprof"
)

var (
	cleanUpFlag = flag.Bool("cleanup", true, "If --cleanup=false is set, "+
		"temporary directories created by this test will be left behind so they can "+
		"be inspected")

	// testingStateDir is a random string generated on test startup, that contains
	// a directory with all of the individual tests' temp directories inside it
	testingStateDir string
)

func RandomName(t testing.TB) string {
	t.Helper()
	buf := bytes.Buffer{}
	if err := binary.Write(&buf, binary.LittleEndian, rand.Uint64()); err != nil {
		t.Fatalf("could not generate random dir name: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf.Bytes())
}

// GetTestDir creates a new temporary directory inside ./'testingStateDir' and
// returns its name. If creating the directory fails, this calls t.Fatal().
func GetTestDir(t testing.TB) string {
	t.Helper()

	// Get the name of the calling test
	testName := "TestAnonymous" + RandomName(t)
	pcs := make([]uintptr, 128) // 128 stack frames
	pcs = pcs[:runtime.Callers(2, pcs)]
	frames := runtime.CallersFrames(pcs)
	more := true
	for more {
		var frame runtime.Frame
		frame, more = frames.Next()
		if frame.Func == nil {
			continue
		}
		n := frame.Func.Name()
		n = n[strings.LastIndexByte(n, '.')+1:]
		if strings.HasPrefix(n, "Test") {
			testName = n
			break
		}
	}

	// Create a directory for that test in 'testingStateDir'
	path := p.Join(testingStateDir, testName)
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatalf("could not create dir %q: %v", path, err)
	}
	return path
}

func StartForTest(t testing.TB, stateDir string) *Watch {
	t.Helper()
	testingStateDir := stateDir + "-state"
	if err := os.Mkdir(testingStateDir, 0755); err != nil {
		t.Fatalf("could not create watch state dir %q: %v", testingStateDir, err)
	}
	w, err := Start(testingStateDir)
	if err != nil {
		t.Fatalf("could not start watch: %v", err)
	}
	return w
}

func CleanUp(t testing.TB, stateDir string) {
	t.Helper()
}

func TestFileCreated(t *testing.T) {
	d := GetTestDir(t)
	defer os.RemoveAll(d)
	w := StartForTest(t, d)

	// Add watch for tmp dir
	w.AddWatch(d, "project")
	touches := make(chan struct{}, 10)
	w.SetCallback(func() {
		touches <- struct{}{}
	})

	// Do file events & watch for touches
	os.Create(j(d, "a"))
	CheckEvent(t, Exactly(1), touches)
}

func TestFileModified(t *testing.T) {
	// Initialize tmp dir
	d := GetTestDir(t)
	defer os.RemoveAll(d)
	w := StartForTest(t, d)

	os.Create(j(d, "a"))

	// Add watch for tmp dir
	w.AddWatch(d, "project")
	touches := make(chan struct{}, 10)
	w.SetCallback(func() {
		touches <- struct{}{}
	})

	// Do file events & watch for touches
	f, err := os.OpenFile(j(d, "a"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("could not open %q for writing: %v", j(d, "a"), err)
	}
	f.WriteString("This is a test")
	CheckEvent(t, Exactly(1), touches)
}

func TestFileDeleted(t *testing.T) {
	// Initialize tmp dir
	d := GetTestDir(t)
	defer os.RemoveAll(d)
	w := StartForTest(t, d)

	os.Create(j(d, "a"))

	// Add watch for tmp dir
	w.AddWatch(d, "project")
	touches := make(chan struct{}, 10)
	w.SetCallback(func() {
		touches <- struct{}{}
	})

	// Do file events & watch for touches
	err := os.Remove(j(d, "a"))
	if err != nil {
		t.Fatalf("could not delete %q: %v", j(d, "a"), err)
	}
	CheckEvent(t, Exactly(1), touches)
}

func TestFileMoved(t *testing.T) {
	// Initialize tmp dir
	d := GetTestDir(t)
	defer os.RemoveAll(d)
	w := StartForTest(t, d)

	os.Create(j(d, "a"))

	// Add watch for tmp dir
	w.AddWatch(d, "project")
	touches := make(chan struct{}, 10)
	w.SetCallback(func() {
		touches <- struct{}{}
	})

	// Do file events & watch for touches
	err := os.Rename(j(d, "a"), j(d, "b"))
	if err != nil {
		t.Fatalf("could not move %q to %q: %v", j(d, "a"), j(d, "b"), err)
	}
	CheckEvent(t, Exactly(1), touches)
}

func TestChildDirCreated(t *testing.T) {
	// Initialize tmp dir
	d := GetTestDir(t)
	defer os.RemoveAll(d)
	w := StartForTest(t, d)

	// Add watch for tmp dir
	w.AddWatch(d, "project")
	touches := make(chan struct{}, 10)
	w.SetCallback(func() {
		touches <- struct{}{}
	})

	// Create child directory
	if err := os.Mkdir(j(d, "d"), 0755); err != nil {
		t.Fatalf("could not make dir %q: %v", j(d, "d"), err)
	}
	CheckEvent(t, Exactly(1), touches)

	// Do file events & watch for touches
	f, err := os.OpenFile(j(d, "d", "a"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("could not create %q: %v", j(d, "d", "a"), err)
	}
	CheckEvent(t, Exactly(1), touches)

	_, err = f.WriteString("This is a test")
	if err != nil {
		t.Fatalf("could not write to %q: %v", j(d, "d", "a"), err)
	}
	err = f.Sync()
	if err != nil {
		t.Fatalf("could not sync %q: %v", j(d, "d", "a"), err)
	}
	CheckEvent(t, Exactly(1), touches)
}

func TestChildDirDeleted(t *testing.T) {
	// Initialize tmp dir
	d := GetTestDir(t)
	defer os.RemoveAll(d)
	w := StartForTest(t, d)

	// Add watch for tmp dir
	w.AddWatch(d, "project")
	touches := make(chan struct{}, 10)
	w.SetCallback(func() {
		touches <- struct{}{}
	})

	// Create child directory
	if err := os.Mkdir(j(d, "d"), 0755); err != nil {
		t.Fatalf("could not make dir %q: %v", j(d, "d"), err)
	}
	CheckEvent(t, Exactly(1), touches)

	f, err := os.OpenFile(j(d, "d", "a"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("could not create %q: %v", j(d, "d", "a"), err)
	}
	_, err = f.WriteString("This is a test")
	if err != nil {
		t.Fatalf("could not write to %q: %v", j(d, "d", "a"), err)
	}
	err = f.Sync()
	if err != nil {
		t.Fatalf("could not sync %q: %v", j(d, "d", "a"), err)
	}
	CheckEvent(t, Exactly(1), touches) // events will be batched into one event

	// Delete the child dir, and make sure the event is registered
	fmt.Printf("about to remove %q\n", j(d, "d"))
	if err := os.RemoveAll(j(d, "d")); err != nil {
		t.Fatalf("could not remove %q: %v", j(d, "d"), err)
	}
	CheckEvent(t, Exactly(1), touches)

	// Make sure w's internal maps were updated
	if len(w.wdToPath) != 1 {
		t.Fatalf("w should be watching one dir, but is watching %d: %v", len(w.wdToPath), w.wdToPath)
	}
}

func TestChildDirMoved(t *testing.T) {
	// Initialize tmp dir
	d := GetTestDir(t)
	defer os.RemoveAll(d)
	w := StartForTest(t, d)

	// Add watch for tmp dir
	w.AddWatch(d, "project")
	touches := make(chan struct{}, 10)
	w.SetCallback(func() {
		touches <- struct{}{}
	})

	// Create child directory
	if err := os.Mkdir(j(d, "e"), 0755); err != nil {
		t.Fatalf("could not make dir %q: %v", j(d, "d"), err)
	}
	CheckEvent(t, Exactly(1), touches)

	// Move child directory
	if err := os.Rename(j(d, "e"), j(d, "d")); err != nil {
		t.Fatalf("could not make dir %q: %v", j(d, "d"), err)
	}
	CheckEvent(t, Exactly(1), touches)

	// Do file events & watch for touches
	f, err := os.OpenFile(j(d, "d", "a"), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("could not create %q: %v", j(d, "d", "a"), err)
	}
	CheckEvent(t, Exactly(1), touches)

	_, err = f.WriteString("This is a test")
	if err != nil {
		t.Fatalf("could not write to %q: %v", j(d, "d", "a"), err)
	}
	err = f.Sync()
	if err != nil {
		t.Fatalf("could not sync %q: %v", j(d, "d", "a"), err)
	}
	CheckEvent(t, Exactly(1), touches)
}
func TestRootDirMoved(t *testing.T) {
}
func TestRootDirDeleted(t *testing.T) {
}

// TestDeleteDirTree deletes an entire directory tree, and then makes sure that
// the corresponding watch descriptors are removed from the watch's internal
// maps
func TestDeleteDirTree(t *testing.T) {
}

func TestMain(m *testing.M) {
	// parse --nocleanup and others
	flag.Parse()
	// pprof
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	var err error
	testingStateDir, err = ioutil.TempDir(".", "watch-test-")
	if err != nil {
		panic(fmt.Sprintf("could not create tmp dir: %v", err))
	}
	if *cleanUpFlag {
		defer os.RemoveAll(testingStateDir) // defer ensures this happens after panic
	}
	errCode := m.Run()
	os.Exit(errCode)
}
