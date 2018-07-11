package status

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"time"
)

var (
	// maxTickGap is the amount of time such that if the last tick is farther than
	// this in the past, the previous time entry will be stopped
	maxTickGap = 24 * time.Minute
)

type status struct {
	// The directory where tg is storing its status
	statusDir string

	// latestTick is the last time a write was registered in a project directory
	latestTick time.Time
	// projectName is name of the toggl project with which the most recently
	// registered write was associated (used by `tg tick`)
	projectName string
	// projectID is ID of the same toggl project
	projectID string
}

func (s *status) MarshalJSON() ([]byte, error) {
	output := map[string]string{
		"tick":         s.latestTick.Format(time.RFC3339),
		"project_name": s.projectName,
		"project_id":   s.projectID,
	}
	return json.Marshal(output)
}

func (s *status) UnmarshalJSON(data []byte) error {
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

func CurrentStatus(statusDir string) (*status, error) {
	if _, err := os.Stat(statusDir); err != nil {
		return nil, fmt.Errorf("could not stat status directory at %q: %v", statusDir, err)
	}
	tickFile := path.Join(statusDir, "tick")
	f, err := os.Open(tickFile)
	if err != nil {
		return nil, err
	}
	result := &status{
		statusDir: statusDir,
	}
	if err := json.NewDecoder(f).Decode(result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *status) Save() error {
	if _, err := os.Stat(s.statusDir); err != nil {
		if err := os.MkdirAll(s.statusDir, 0755); err != nil {
			return fmt.Errorf("could not create state dir at %q: %v", s.statusDir, err)
		}
	}
	tickFile := path.Join(s.statusDir, "tick")
	f, err := os.OpenFile(tickFile, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return fmt.Errorf("could not create status file at %q: %v", tickFile, err)
	}
	return json.NewEncoder(f).Encode(s)
}

func (s *status) Tick(projectName string) error {
	now := time.Now()
	if now.Sub(s.latestTick) > maxTickGap {
		s.Stop(s.latestTick)
	}
	s.latestTick = now
	s.projectName = projectName
	// TODO look up project ID
	return s.Save()
}

func (s *status) Stop(t time.Time) error {
	req, err := http.NewRequest("PUT",
		fmt.Sprintf("https://www.toggl.com/api/v8/time_entries/%s/stop"), nil)
	if err != nil {
		return fmt.Errorf("could not construct stop request: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	fmt.Printf("%+v\n", resp)
	return err
}
