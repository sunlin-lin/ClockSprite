package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

var activeReminder row
var displayedReminderCount int

func completeRow(selected row) {
	switch selected.action {
	case "clock":
		if !slices.Contains(clock.Pending, selected.id) {
			return
		}
		next := clock
		next.Pending = slices.DeleteFunc(slices.Clone(next.Pending), func(key string) bool { return key == selected.id })
		next.Acknowledged = maps.Clone(clock.Acknowledged)
		next.Acknowledged[selected.id] = true
		if err := writeClock(next); err != nil {
			notice(err.Error())
			return
		}
	case "complete", "restore":
		next := checklist.data
		next.Items = slices.Clone(next.Items)
		index := slices.IndexFunc(next.Items, func(entry item) bool { return entry.Rule == selected.id && entry.occurrence() == selected.day })
		if index < 0 {
			return
		}
		next.Items[index].Completed = selected.action == "complete"
		if err := checklist.commit(next); err != nil {
			notice(err.Error())
			return
		}
	default:
		notice("請選取待打卡、待完成或已完成的項目。")
		return
	}
	populate()
}

func refreshReminder(pending []row) {
	var next row
	count := 0
	for _, entry := range pending {
		if entry.action != "complete" && (entry.action != "clock" || clock.QuietClock) {
			continue
		}
		count++
		if next.action == "" || entry == activeReminder {
			next = entry
		}
	}
	if activeReminder == next && displayedReminderCount == count {
		return
	}
	wasActive := activeReminder.action != ""
	activeReminder = next
	displayedReminderCount = count
	if count == 0 {
		hideReminderBubble()
		showMascot(!clock.Hidden)
		if !clock.Hidden {
			if lastQuote < 0 {
				speak(time.Now())
			} else {
				paintMascot(lastQuote)
			}
		}
		return
	}
	title, detail := reminderText(next, count)
	if !wasActive {
		paintMascot(-1)
		showMascot(true)
	}
	showReminderBubble(title, detail)
}

func reminderText(entry row, count int) (string, string) {
	counter := fmt.Sprintf("待確認提醒 · %d 項", count)
	if entry.action == "clock" {
		date, kind, _ := strings.Cut(entry.id, "/")
		title := "請記得上班打卡"
		if kind == "1" {
			title = "請記得下班打卡"
		}
		return title, date + "  ·  " + counter
	}
	for _, item := range checklist.data.Items {
		if item.Rule == entry.id && item.occurrence() == entry.day {
			return item.Title, item.Day + "  " + item.Time + "  ·  " + counter
		}
	}
	return entry.label, counter
}

// Keep occurrence keys so the scheduler does not recreate a cleared item.
func clearCompleted(next state) state {
	next.Cleared = maps.Clone(next.Cleared)
	if next.Cleared == nil {
		next.Cleared = map[string]bool{}
	}
	next.Items = slices.DeleteFunc(slices.Clone(next.Items), func(entry item) bool {
		if !entry.Completed {
			return false
		}
		next.Cleared[entry.Rule+"/"+entry.occurrence()] = true
		return true
	})
	return next
}
