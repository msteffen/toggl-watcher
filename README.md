# toggl-watcher (tg)

Command line tool to track time in Toggl based on local filesystem writes

This utility uses (or will use, when I implement it) inotify to watch
directories under one or more user-specified project directories (e.g.
`/home/msteffen/go/src/github.com/msteffen/toggl-watcher`) for writes. If this
tool observes a write, it will start a new time entry in Toggl (in case I
forget). If more than 24 minutes elapse without any writes, this tool stops the
last time entry in toggl that it created and shortens it to end at the last
write

Some features that I hope to support:

- If you turn your computer off and then resume watching project directories
  much later, toggle-watcher should go back and fix all of your long-running
  time entries.

- If you start a time entry with toggl-watcher, and then end that entry via the
  toggl website and start a new time entry, toggl-watcher shouldn't interfere
  with the manually

---

**Update**: I've kind of abanoned the main goal of this project for now and am
spending all of my time refining the inotify library in findtest, which is
proving to be the hardest part. I haven't tested it, but I'm not convinced that
go libraries (`fdnotify` in particular) handle the racy parts of this well
(i.e. ensuring that a stream of updates is generated that is complete—i.e.
replaying it in another location would be sufficient to clone a watched
directory). I should test this, though.
