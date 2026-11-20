package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type clockSettings struct {
	Times       [2]string
	Minutes     int
	Hidden      bool
	QuietClock  bool
	Mascot      string
	SpriteScale int
}
type clockLog struct {
	Pending      []string
	Acknowledged map[string]bool
}
type calendarCache struct {
	Days    map[string]bool
	Updated string
}
type reminderSettings struct{ Rules []rule }
type reminderLog struct {
	Items   []item
	Scanned time.Time
	Cleared map[string]bool
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		err = os.WriteFile(path+".tmp", raw, 0600)
	}
	if err == nil {
		err = os.Rename(path+".tmp", path)
	}
	return err
}
func readStored(path string, value any) (bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = json.Unmarshal(raw, value); err != nil {
		return false, fmt.Errorf("無法讀取 %s：%w", path, err)
	}
	return true, nil
}
func (a *app) commit(next state) error {
	if err := writeJSON(a.path, reminderLog{next.Items, next.Scanned, next.Cleared}); err != nil {
		return err
	}
	a.data = next
	return nil
}
func (a *app) saveRules(next state) error {
	if err := writeJSON(a.rulesPath, reminderSettings{next.Rules}); err != nil {
		return err
	}
	a.data = next
	return nil
}
func writeClock(next clockState) error {
	if err := writeJSON(clockPath, clockLog{next.Pending, next.Acknowledged}); err != nil {
		return err
	}
	clock = next
	return nil
}
func saveClockSettings(next clockState) error {
	if err := writeJSON(settingsPath, clockSettings{next.Times, next.Minutes, next.Hidden, next.QuietClock, next.Mascot, next.SpriteScale}); err != nil {
		return err
	}
	clock = next
	return nil
}
func saveCalendarCache(next clockState) error {
	if err := writeJSON(calendarPath, calendarCache{next.Days, next.Updated}); err != nil {
		return err
	}
	clock = next
	return nil
}
func load() (*os.File, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(root, "ClockSprite")
	legacyDir := filepath.Join(base, "Checklist")
	if err = os.MkdirAll(legacyDir, 0700); err != nil {
		return nil, err
	}
	lock, err := acquireLock(filepath.Join(legacyDir, "instance.lock"))
	if err != nil {
		return nil, err
	}
	if err = loadStores(base, legacyClockPath(root), filepath.Join(legacyDir, "checklist.json")); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

func loadStores(base, oldClock, oldChecklist string) error {
	settingsPath = filepath.Join(base, "Settings", "settings.json")
	clockPath = filepath.Join(base, "Logs", "clock.json")
	calendarPath = filepath.Join(base, "Cache", "calendar.json")
	checklist = app{path: filepath.Join(base, "Logs", "reminders.json"), rulesPath: filepath.Join(base, "Settings", "reminders.json")}
	for _, dir := range []string{"Settings", "Logs", "Cache"} {
		if err := os.MkdirAll(filepath.Join(base, dir), 0700); err != nil {
			return err
		}
	}
	clock = clockState{Times: [2]string{"08:30", "18:00"}, Minutes: 60, Days: map[string]bool{}, Acknowledged: map[string]bool{}}
	for _, year := range []int{2026, 2027} {
		raw, _ := resources.ReadFile(fmt.Sprintf("%d.csv", year))
		if days, err := parseCalendar(raw, year); err == nil {
			for key, value := range days {
				clock.Days[key] = value
			}
		}
	}
	var prefs clockSettings
	var history clockLog
	var calendar calendarCache
	var rules reminderSettings
	var occurrences reminderLog
	paths := []string{settingsPath, clockPath, calendarPath, checklist.rulesPath, checklist.path}
	values := []any{&prefs, &history, &calendar, &rules, &occurrences}
	present := make([]bool, len(paths))
	for i, path := range paths {
		found, err := readStored(path, values[i])
		if err != nil {
			return err
		}
		present[i] = found
	}
	if !present[0] || !present[1] || !present[2] {
		if _, err := readStored(oldClock, &clock); err != nil {
			return err
		}
	}
	if !present[3] || !present[4] {
		if _, err := readStored(oldChecklist, &checklist.data); err != nil {
			return err
		}
	}
	if present[0] {
		clock.Times, clock.Minutes, clock.Hidden, clock.QuietClock = prefs.Times, prefs.Minutes, prefs.Hidden, prefs.QuietClock
		clock.Mascot, clock.SpriteScale = prefs.Mascot, prefs.SpriteScale
	}
	if present[1] {
		clock.Pending, clock.Acknowledged = history.Pending, history.Acknowledged
	}
	if present[2] {
		clock.Days, clock.Updated = calendar.Days, calendar.Updated
	}
	if present[3] {
		checklist.data.Rules = rules.Rules
	}
	if present[4] {
		checklist.data.Items, checklist.data.Scanned, checklist.data.Cleared = occurrences.Items, occurrences.Scanned, occurrences.Cleared
	}
	if clock.Days == nil {
		clock.Days = map[string]bool{}
	}
	if clock.Acknowledged == nil {
		clock.Acknowledged = map[string]bool{}
	}
	if clock.Minutes < 1 || clock.Minutes > 1440 {
		clock.Minutes = 60
	}
	// A deleted rule can leave a pending occurrence on disk up to the next checkpoint.
	enabled := map[string]bool{}
	for _, task := range checklist.data.Rules {
		enabled[task.ID] = true
	}
	checklist.data.Items = slices.DeleteFunc(checklist.data.Items, func(entry item) bool { return !entry.Completed && !enabled[entry.Rule] })
	initialize := []any{
		clockSettings{clock.Times, clock.Minutes, clock.Hidden, clock.QuietClock, clock.Mascot, clock.SpriteScale}, clockLog{clock.Pending, clock.Acknowledged},
		calendarCache{clock.Days, clock.Updated}, reminderSettings{checklist.data.Rules}, reminderLog{checklist.data.Items, checklist.data.Scanned, checklist.data.Cleared},
	}
	for i, path := range paths {
		if !present[i] {
			if err := writeJSON(path, initialize[i]); err != nil {
				return err
			}
		}
	}
	return nil
}
