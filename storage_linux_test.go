package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitStorageMigrationAndWriteIsolation(t *testing.T) {
	dir := t.TempDir()
	oldClock := filepath.Join(dir, "native-state.json")
	oldChecklist := filepath.Join(dir, "checklist.json")
	old := clockState{Times: [2]string{"08:45", "18:15"}, Minutes: 30, Hidden: true,
		Pending: []string{"2026-11-19/1"}, Acknowledged: map[string]bool{"2026-11-19/0": true},
		Days: map[string]bool{"2026-11-19": true}, Updated: "2026-11-19"}
	prior := state{Rules: []rule{{ID: "weekly", Title: "寄週報"}}, Items: []item{
		{Rule: "weekly", Day: "2026-11-19", Completed: false},
		{Rule: "deleted", Day: "2026-11-18", Completed: true},
	}, Scanned: at(t, "2026-11-19 08:30")}
	for path, data := range map[string]any{oldClock: old, oldChecklist: prior} {
		if err := writeJSON(path, data); err != nil {
			t.Fatal(err)
		}
	}
	base := filepath.Join(dir, "app")
	if err := loadStores(base, oldClock, oldChecklist); err != nil {
		t.Fatal(err)
	}
	if clock.Times != old.Times || !clock.Hidden || len(clock.Pending) != 1 || !clock.Acknowledged["2026-11-19/0"] || !clock.Days["2026-11-19"] || clock.Updated != old.Updated || len(checklist.data.Items) != 2 || len(checklist.data.Rules) != 1 {
		t.Fatal("migration lost settings, calendar, or history")
	}
	read := func(path string) string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	prefsBefore, rulesBefore, calendarBefore := read(settingsPath), read(checklist.rulesPath), read(calendarPath)
	if strings.Contains(prefsBefore, "Pending") || strings.Contains(rulesBefore, "Items") || strings.Contains(read(clockPath), "Times") {
		t.Fatal("data types still mixed")
	}
	next := clock
	next.Pending = nil
	if err := writeClock(next); err != nil {
		t.Fatal(err)
	}
	log := checklist.data
	log.Scanned = log.Scanned.AddDate(0, 0, 1)
	if err := checklist.commit(log); err != nil {
		t.Fatal(err)
	}
	if read(settingsPath) != prefsBefore || read(checklist.rulesPath) != rulesBefore || read(calendarPath) != calendarBefore {
		t.Fatal("recording history changed settings or cache")
	}
	if err := os.WriteFile(oldClock, []byte("old file is now invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := loadStores(base, oldClock, oldChecklist); err != nil {
		t.Fatal(err)
	}
	if len(clock.Pending) != 0 || !checklist.data.Scanned.Equal(log.Scanned) {
		t.Fatal("legacy values incorrectly restored")
	}
}
