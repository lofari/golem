// internal/ctx/log.go
package ctx

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Log struct {
	Sessions []Session `yaml:"sessions"`
}

type Session struct {
	Iteration     int      `yaml:"iteration"`
	Timestamp     string   `yaml:"timestamp"`
	Task          string   `yaml:"task"`
	Outcome       string   `yaml:"outcome"`
	Summary       string   `yaml:"summary"`
	FilesChanged  []string `yaml:"files_changed,omitempty"`
	DecisionsMade []string `yaml:"decisions_made,omitempty"`
	PitfallsFound []string `yaml:"pitfalls_found,omitempty"`
}

func LogPath(dir string) string {
	return filepath.Join(dir, ".ctx", "log.yaml")
}

func ReadLog(dir string) (Log, error) {
	data, err := os.ReadFile(LogPath(dir))
	if err != nil {
		return Log{}, fmt.Errorf("reading log.yaml: %w", err)
	}
	var l Log
	if err := yaml.Unmarshal(data, &l); err != nil {
		return Log{}, fmt.Errorf("parsing log.yaml: %w", err)
	}
	return l, nil
}

func WriteLog(dir string, l Log) error {
	data, err := yaml.Marshal(&l)
	if err != nil {
		return fmt.Errorf("marshaling log: %w", err)
	}
	return os.WriteFile(LogPath(dir), data, 0644)
}

func AppendSession(dir string, sess Session) error {
	l, err := ReadLog(dir)
	if err != nil {
		return err
	}
	l.Sessions = append(l.Sessions, sess)
	return WriteLog(dir, l)
}

// LastNSessions returns the last n sessions, or all if n <= 0 or n > len.
func (l Log) LastNSessions(n int) []Session {
	if n <= 0 || n >= len(l.Sessions) {
		return l.Sessions
	}
	return l.Sessions[len(l.Sessions)-n:]
}

// FailedSessions returns sessions with outcome "blocked" or "unproductive".
func (l Log) FailedSessions() []Session {
	var result []Session
	for _, s := range l.Sessions {
		if s.Outcome == "blocked" || s.Outcome == "unproductive" {
			result = append(result, s)
		}
	}
	return result
}
