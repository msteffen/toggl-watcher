package main

import (
	"fmt"
	"io"
	"os"
	p "path"
	fp "path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

func main() {
	dirCount := 0

	// Add watches to subdirectories of arg[1]
	err = fp.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error {
		// TODO what if directories and such are added while the watch is being created?
		// TODO what if 'path' is deleted after this fn is called? (err will be non-nil, as I've learned)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error walking %s: %v\n", path, err)
			return err
		}

		// Only watch directories
		if !info.IsDir() {
			return nil
		}
		fmt.Println(path)

		// skip hidden directories
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

		dirCount++
		return nil
	})
	if err != nil {
		fmt.Printf("couldn't walk %q: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	fmt.Printf("dir count: %d\n", dirCount)

	func() {
		buf := make([]byte, 1024*unix.SizeofInotifyEvent) // huge buffer, to hold all events
		for {
			n, err := unix.Read(fd, buf)
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
				fmt.Printf("name: %q\n", name)
				idx += int(event.Len)
				fmt.Printf("%#v\n%s\n", event, Render(event, name))
			}
		}
	}()
}
