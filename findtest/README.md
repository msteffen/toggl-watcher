- This directory has a binary that I'm using to build a library for
  inotify-watching a directory with all of its subdirs
- This is kind of a complicated task because inotify watches take a nontrivial
  amount of time to create, so files/dirs may be created in a watched directory
  in between the watch library noticing it and adding the watch. So you have to
  scan new directories while the watch is starting and deal with files/dirs
  that are already in there.
- The binary here is basically a fast place for me to figure out how to
  architect a library that does this correctly
- The only test is `test_findtest.sh` but I hope to add more:
  - Creating/deleting subdirectories quickly (and making sure all the watches
    and internal data structures are updated appropriately
  - Some kind of fuzz test that creates and deletes stuff and makes sure all
    the right events appear
  - eventually: deleting the root directory/watching multiple root directories.
    If a root directory doesn't see any writes for a certain period, kill it.
    - Would be nice if this watcher just ran as a daemon process, even.
