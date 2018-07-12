package main

import (
	"fmt"
	"os"
	"path"

	"github.com/msteffen/toggl-watcher/status"
	"github.com/spf13/cobra"
)

const (
	statusDirectoryEnvVar = "TOGGL_WATCHER_DIRECTORY"
	watchesDirectory      = "watches"
)

// statusDir is the directory where toggl-tool keeps its state. May be set to a
// temporary directory for tests
var statusDir = func() string {
	if dir, ok := os.LookupEnv(statusDirectoryEnvVar); ok {
		return dir
	}
	return path.Join(os.Getenv("HOME"), ".toggle-tool")
}()

func resume() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume watching directories for writes (should run on startup)",
		Long: "Resume runs in the background, watching the directories indicated " +
			"in %s/%s for writes and either ends/continues the associated Toggl " +
			"time entries",
		Run: BoundedCommand(0, 0, func(_ []string) error { return nil }),
	}
}

func watch() *cobra.Command {
	return &cobra.Command{
		Use:   "watch <project> <directory>",
		Short: "Begin watching a new project directory",
		Long: "Begin watching <directory> for writes, and use those writes to " +
			"create time events in <project> (if there is any existing project with " +
			"the same name modulo case, that project will be reused, otherwise a new " +
			"toggl project will be created)",
		Run: BoundedCommand(0, 0, func(_ []string) error { return nil }),
	}
}

func tick() *cobra.Command {
	return &cobra.Command{
		Use:   "tick <project>",
		Short: "Note work on a project (same as receiving a write notification)",
		Long:  "Advance the \"working\" timestamp, and possibly switch projects",
		Run: BoundedCommand(1, 1, func(args []string) error {
			s, err := status.Read(statusDir)
			if err != nil {
				return err
			}
			return s.Tick(args[0])
		}),
	}
}

func main() {
	rootCommand := &cobra.Command{
		Use:   "tg",
		Short: "track time in toggl by watching project directories with inotify",
		Long: "tg uses inotify to watch directories that you indicate (in which " +
			"you're doing work). Based on writes under those dirs, tg creates and " +
			"updates projects and time entries in toggl",
	}
	rootCommand.AddCommand(tick())
	rootCommand.AddCommand(watch())
	rootCommand.AddCommand(resume())
	if err := rootCommand.Execute(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

}
