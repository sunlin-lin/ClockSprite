package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var inputs = map[int]string{}
var days [7]bool
var modeOn bool
var cursor int

func setInput(n int, value string) { inputs[n] = value }
func input(n int) string           { return inputs[n] }
func setChecked(n int, b bool)     { days[n] = b }
func checked(n int) bool           { return days[n] }
func weeklyMode() bool             { return modeOn }
func setMode(v bool)               { modeOn = v }
func selected() int                { return cursor }
func setRows([]string, int)        {}
func notice(string)                {}
func modeChanged()                 {}
func showSettings()                {}
func showMascot(on bool)           { mascotVisible = on }
func paintMascot(int)              {}
func setStatus(string)             {}

func acquireLock(s string) (*os.File, error) { return os.Create(s) }
func legacyClockPath(root string) string     { return filepath.Join(root, "ClockSprite", "state.json") }
func at(t *testing.T, s string) time.Time {
	t.Helper()
	result, err := time.ParseInLocation("2006-01-02 15:04", s, taipei)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestIntegratedChecklist(t *testing.T) {
	now := at(t, "2026-11-19 08:30")
	checklist = app{path: filepath.Join(t.TempDir(), "checklist-log.json"), rulesPath: filepath.Join(t.TempDir(), "rules.json"), data: state{Scanned: now}}
	editing = ""
	setInput(fieldTitle, "週報")
	setInput(fieldTime, "09:30")
	setMode(true)
	setChecked(4, true)
	setChecked(0, true)
	if err := saveRule(now); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(checklist.data.Rules[0].Days, []int{0, 4}) {
		t.Fatal("weekly days")
	}
	newRule()
	setInput(fieldTitle, "交資料")
	setInput(fieldDate, "2026-11-20")
	setInput(fieldTime, "10:00")
	if err := saveRule(now); err != nil {
		t.Fatal(err)
	}
	next, added := advance(checklist.data, at(t, "2026-11-22 10:00"))
	if !added || len(next.Items) != 3 {
		t.Fatal("expected two weekly and one date occurrence", next.Items)
	}
	if err := checklist.commit(next); err != nil {
		t.Fatal(err)
	}
	clock.Pending = []string{"2026-11-22/0"}
	populate()
	if rows[0].action != "clock" || rows[1].action != "complete" {
		t.Fatal("clock and custom tasks must share list")
	}
	cursor = 1
	onAction(actionDone)
	if !checklist.data.Items[0].Completed {
		t.Fatal("completion wasn't saved")
	}
	next, added = advance(checklist.data, at(t, "2026-11-26 10:00"))
	if !added || len(next.Items) != 4 || !next.Items[0].Completed {
		t.Fatal("weekly recurrence or completed state lost")
	}
}
func TestCalendarAssets(t *testing.T) {
	for _, year := range []struct {
		name  string
		value int
	}{{"2026.csv", 2026}, {"2027.csv", 2027}} {
		raw, err := resources.ReadFile(year.name)
		if err != nil {
			t.Fatal(err)
		}
		entries, err := parseCalendar(raw, year.value)
		if err != nil || len(entries) != 365 {
			t.Fatalf("invalid calendar: %v", err)
		}
	}
	for _, name := range []string{"art/floating_mascot.png", "art/tray_mascot.png", "art/bubbles/099.png"} {
		if _, err := resources.ReadFile(name); err != nil {
			t.Fatal(err)
		}
	}
}

func renderCalendar(string, [42]string, [42]bool) {}
func showCalendar(bool)                           {}
func autostartChecked() bool                      { return false }
func refreshAutostart()                           {}
func setAutostart(bool) error                     { return nil }

func TestCalendarPickerLeapYear(t *testing.T) {
	february := at(t, "2028-02-14 12:00")
	grid := monthGrid(february)
	if grid[0].Weekday() != time.Sunday || grid[2].Format("2006-01-02") != "2028-02-01" {
		t.Fatal("incorrect month starting column")
	}
	if grid[30].Format("2006-01-02") != "2028-02-29" || grid[31].Format("2006-01-02") != "2028-03-01" {
		t.Fatal("leap day or following month incorrect")
	}
}

func TestClearLeavesSavedRules(t *testing.T) {
	checklist.data = state{Rules: []rule{{ID: "keep", Title: "已儲存"}}}
	setInput(fieldTitle, "未儲存")
	setInput(fieldDate, "2026-11-30")
	setInput(fieldTime, "09:00")
	setMode(true)
	setChecked(3, true)
	onAction(actionClear)
	if len(checklist.data.Rules) != 1 || input(fieldTitle) != "" || input(fieldDate) != "" || input(fieldTime) != "" || checked(3) || weeklyMode() {
		t.Fatal("clear should reset only the form")
	}
}

func TestCalendarSyncOnLateLogin(t *testing.T) {
	before := at(t, "2026-11-19 08:59")
	after := at(t, "2026-11-19 09:00")
	if shouldSyncCalendar(before, "2026-11-18", true, false, time.Time{}) {
		t.Fatal("09:00 sync fired early")
	}
	if !shouldSyncCalendar(after.UTC(), "2026-11-18", true, false, time.Time{}) {
		t.Fatal("late login did not catch up in Taipei time")
	}
	if shouldSyncCalendar(after, "2026-11-19", true, false, time.Time{}) {
		t.Fatal("already synced today")
	}
	if shouldSyncCalendar(after, "2026-11-18", true, true, time.Time{}) || shouldSyncCalendar(after, "2026-11-18", true, false, after.Add(time.Minute)) {
		t.Fatal("parallel sync or early retry")
	}
	if !shouldSyncCalendar(before, "", false, false, time.Time{}) {
		t.Fatal("missing calendar should fetch immediately")
	}
}

var bubbleTitle string
var mascotVisible bool

func showReminderBubble(title, _ string) { bubbleTitle = title }
func hideReminderBubble()                { bubbleTitle = "" }

func TestConfirmFromMascot(t *testing.T) {
	now := at(t, "2026-11-19 08:30")
	checklist = app{path: filepath.Join(t.TempDir(), "checklist.json"), data: state{Scanned: now, Items: []item{
		{Rule: "remind", Day: "2026-11-19", Time: "08:30", Title: "寄週報"},
	}}}
	clockPath = filepath.Join(t.TempDir(), "state.json")
	clock = clockState{Hidden: true, Pending: []string{"2026-11-19/0"}, Acknowledged: map[string]bool{}}
	activeReminder = row{}
	displayedReminderCount = 0
	populate()
	if bubbleTitle != "請記得上班打卡" || !mascotVisible || displayedReminderCount != 2 {
		t.Fatal("mascot should surface first pending reminder")
	}
	onAction(actionConfirmReminder)
	if !clock.Acknowledged["2026-11-19/0"] || bubbleTitle != "寄週報" || displayedReminderCount != 1 {
		t.Fatal("confirm must only close the visible task")
	}
	onAction(actionConfirmReminder)
	if !checklist.data.Items[0].Completed || bubbleTitle != "" || mascotVisible {
		t.Fatal("last confirmation must restore hidden state")
	}
}

func setHistoryView(bool) {}

func TestHistoryIsSeparateAndRemindersKeepWorking(t *testing.T) {
	showHistory = false
	t.Cleanup(func() { showHistory = false })
	clock = clockState{}
	checklist = app{path: filepath.Join(t.TempDir(), "logs.json"), data: state{
		Rules: []rule{{ID: "daily", Title: "站會"}},
		Items: []item{
			{Rule: "daily", Day: "2026-11-18", Title: "站會", Completed: true},
			{Rule: "daily", Day: "2026-11-19", Title: "站會"},
		},
	}}
	activeReminder = row{}
	displayedReminderCount = 0
	populate()
	if len(rows) != 2 || rows[0].action != "complete" || rows[1].action != "edit" {
		t.Fatal("main list must omit history")
	}
	onAction(actionShowHistory)
	if len(rows) != 1 || rows[0].action != "restore" || bubbleTitle != "站會" {
		t.Fatal("history must omit pending, but active reminder should continue")
	}
	cursor = 0
	onAction(actionDone)
	if len(rows) != 0 || checklist.data.Items[0].Completed {
		t.Fatal("cancel completion should remove the history entry")
	}
	onAction(actionShowPending)
	if len(rows) != 3 {
		t.Fatal("restored item must return to the main list")
	}
}

func forceClockChecked() bool   { return true }
func setForceClockChecked(bool) {}
func confirmClearHistory() bool { return true }

func TestQuietClockDoesNotSuppressCustomReminder(t *testing.T) {
	showHistory = false
	clock = clockState{Hidden: true, QuietClock: true, Pending: []string{"2026-11-19/0"}}
	checklist = app{}
	activeReminder = row{}
	displayedReminderCount = 0
	populate()
	if len(rows) != 1 || activeReminder.action != "" {
		t.Fatal("clock must stay visible in the list only")
	}
	checklist.data.Items = []item{{Rule: "weekly", Day: "2026-11-19", Title: "站會"}}
	populate()
	if bubbleTitle != "站會" || displayedReminderCount != 1 {
		t.Fatal("custom reminder did not pop up")
	}
}
func TestClearedHistoryDoesNotReschedule(t *testing.T) {
	now := at(t, "2026-11-19 09:20")
	prior := state{Rules: []rule{{ID: "weekly", Days: []int{4}, Time: "09:00"}}, Scanned: now,
		Items: []item{{Rule: "weekly", Day: "2026-11-19", Completed: true}, {Rule: "pending", Day: "2026-11-19"}},
	}
	cleared := clearCompleted(prior)
	if len(cleared.Rules) != 1 || len(cleared.Items) != 1 || cleared.Items[0].Rule != "pending" || len(prior.Items) != 2 {
		t.Fatal("clear modified pending data or rules")
	}
	later, added := advance(cleared, now.Add(time.Minute))
	if added || len(later.Items) != 1 {
		t.Fatal("cleared reminder reappeared")
	}
	_, added = advance(later, now.AddDate(0, 0, 7))
	if !added {
		t.Fatal("future weekly occurrence did not run")
	}
}

func showAppearance()           {}
func reloadSprite()             {}
func setAppearanceLabel(string) {}

func TestAllQuoteBubblesPackaged(t *testing.T) {
	entries, err := resources.ReadDir("art/bubbles")
	if err != nil || len(entries) != 300 {
		t.Fatalf("want 300 quote bubbles: got %d, %v", len(entries), err)
	}
	for i, entry := range entries {
		name := entry.Name()
		if name != fmt.Sprintf("%03d.png", i) {
			t.Fatalf("unexpected quote asset %s", name)
		}
	}
}
