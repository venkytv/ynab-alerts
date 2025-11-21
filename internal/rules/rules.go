package rules

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Rule represents a rule definition loaded from YAML.
type Rule struct {
	Name    string      `yaml:"name"`
	Observe ObserveList `yaml:"observe,omitempty"`
	When    WhenList    `yaml:"when"`
	Notify  []string    `yaml:"notify"`
	Meta    interface{} `yaml:"meta,omitempty"`
}

// Observe captures a value under a named variable on a schedule.
type Observe struct {
	CaptureOn string `yaml:"capture_on"` // day-of-month (1-31) or empty for every run
	Variable  string `yaml:"variable"`
	Value     string `yaml:"value"` // expression to evaluate to a money value
}

// When describes the evaluation condition for a rule.
type When struct {
	Window     string   `yaml:"window,omitempty"`       // optional textual window; best-effort
	DayOfMonth []int    `yaml:"day_of_month,omitempty"` // restrict evaluation to these days (1-31)
	DaysOfWeek []string `yaml:"days_of_week,omitempty"` // restrict to weekdays (Mon-Sun)
	NthWeekday string   `yaml:"nth_weekday,omitempty"`  // e.g., "1 Monday", "last Friday"
	Schedule   string   `yaml:"schedule,omitempty"`     // cron-like "min hour dom mon dow"
	Condition  string   `yaml:"condition,omitempty"`    // expression returning bool
}

// ObserveList allows single-object or list YAML.
type ObserveList []Observe

// UnmarshalYAML custom unmarshals a single observe or a list.
func (o *ObserveList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var seq []Observe
	if err := unmarshal(&seq); err == nil {
		*o = seq
		return nil
	}
	var single Observe
	if err := unmarshal(&single); err == nil {
		*o = []Observe{single}
		return nil
	}
	return fmt.Errorf("observe must be object or list")
}

// WhenList allows single-object or list YAML.
type WhenList []When

// UnmarshalYAML custom unmarshals a single when or a list.
func (w *WhenList) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var seq []When
	if err := unmarshal(&seq); err == nil {
		*w = seq
		return nil
	}
	var single When
	if err := unmarshal(&single); err == nil {
		*w = []When{single}
		return nil
	}
	return fmt.Errorf("when must be object or list")
}

// Data is the evaluation context.
type Data struct {
	Accounts map[string]int64
	Vars     map[string]int64
	Now      time.Time
}

// Trigger represents a fired rule.
type Trigger struct {
	Rule    Rule
	Message string
}

// LoadDir reads all YAML files in the directory into a rule slice.
func LoadDir(dir string) ([]Rule, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var rules []Rule
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch filepath.Ext(entry.Name()) {
		case ".yaml", ".yml":
		default:
			continue
		}

		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var fileRules []Rule
		if err := yaml.Unmarshal(content, &fileRules); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		rules = append(rules, fileRules...)
	}
	if len(rules) == 0 {
		return nil, errors.New("no rule files found")
	}
	return rules, nil
}
