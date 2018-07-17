// Package status is an internal library for interacting with Toggl.
package status

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"
)

const (
	tickFile = "tick"
)

var (
	// maxTickGap is the amount of time such that if the last tick is farther than
	// this in the past, the previous time entry will be stopped
	maxTickGap = 24 * time.Minute
)

// Status is the data structure that toggl-watcher uses to track your work
type Status struct {
	// The directory where tg is storing its state
	tgStateDir string

	// latestTick is the last time a write was registered in a project directory
	latestTick time.Time
	// projectName is name of the toggl project with which the most recently
	// registered write was associated (used by `tg tick`)
	projectName string
	// projectID is ID of the same toggl project
	projectID string
	// timeEntryID is the ID of the currently open Toggl time entry (if any)
	timeEntryID string
}

// MarshalJSON allows Status to implement the json.Marshaller interface
func (s *Status) MarshalJSON() ([]byte, error) {
	output := map[string]string{
		"tick":         s.latestTick.Format(time.RFC3339),
		"project_name": s.projectName,
		"project_id":   s.projectID,
	}
	return json.Marshal(output)
}

// UnmarshalJSON allows Status to implement the json.Unmarshaller interface
func (s *Status) UnmarshalJSON(data []byte) error {
	fields := make(map[string]string)
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	s.projectName = fields["project_name"]
	s.projectID = fields["project_id"]
	var err error
	s.latestTick, err = time.Parse(time.RFC3339, fields["tick"])
	if err != nil {
		return fmt.Errorf("could not parse time %q: %v", fields["tick"], err)
	}
	return nil
}

// Read reads the latest tick info from tgStateDir/tick into memory
func Read(tgStateDir string) (*Status, error) {
	if _, err := os.Stat(tgStateDir); err != nil {
		return nil, fmt.Errorf("could not stat status directory at %q: %v", tgStateDir, err)
	}
	tickFile := path.Join(tgStateDir, tickFile)
	f, err := os.Open(tickFile)
	if err != nil {
		return nil, err
	}
	result := &Status{
		tgStateDir: tgStateDir,
	}
	if err := json.NewDecoder(f).Decode(result); err != nil {
		return nil, err
	}
	return result, nil
}

// Save persists 's' to the file 's.tgStateDir/tick
func (s *Status) Save() error {
	if _, err := os.Stat(s.tgStateDir); err != nil {
		if err := os.MkdirAll(s.tgStateDir, 0755); err != nil {
			return fmt.Errorf("could not create state dir at %q: %v", s.tgStateDir, err)
		}
	}
	tickFile := path.Join(s.tgStateDir, tickFile)
	f, err := os.OpenFile(tickFile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("could not create status file at %q: %v", tickFile, err)
	}
	return json.NewEncoder(f).Encode(s)
}

// Tick notifies 's' that a new work event has occurred on the project
// 'projectName'
func (s *Status) Tick(projectName string) error {
	now := time.Now()
	if now.Sub(s.latestTick) > maxTickGap {
		s.Stop(s.latestTick)
	}
	s.latestTick = now
	s.projectName = projectName
	// TODO look up project ID
	return s.Save()
}

// Stop is a helper function that causes 's' to tell toggl that work in the
// current Toggl time event has stopped
func (s *Status) Stop(t time.Time) error {
	resp, err := Post(fmt.Sprintf("time_entries/%s/stop", s.timeEntryID), "")
	fmt.Printf("%+v (%v)\n", resp, err)
	return err
}
