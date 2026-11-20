package main

import (
	"fmt"
	"time"
)

var pickerMonth time.Time

func firstOfMonth(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
}

func monthGrid(month time.Time) [42]time.Time {
	first := firstOfMonth(month)
	start := first.AddDate(0, 0, -int(first.Weekday()))
	var days [42]time.Time
	for i := range days {
		days[i] = start.AddDate(0, 0, i)
	}
	return days
}

func openCalendar() {
	if weeklyMode() {
		return
	}
	day, err := time.ParseInLocation("2006-01-02", input(fieldDate), taipei)
	if err != nil {
		day = time.Now().In(taipei)
	}
	pickerMonth = firstOfMonth(day)
	updateCalendar()
	showCalendar(true)
}

func updateCalendar() {
	days := monthGrid(pickerMonth)
	var labels [42]string
	var enabled [42]bool
	today := localDate(time.Now())
	current := input(fieldDate)
	for i, day := range days {
		if day.Month() != pickerMonth.Month() {
			continue
		}
		labels[i] = fmt.Sprint(day.Day())
		if localDate(day) == current {
			labels[i] = "[" + labels[i] + "]"
		}
		enabled[i] = localDate(day) >= today
	}
	renderCalendar(pickerMonth.Format("2006 年 01 月"), labels, enabled)
}

func calendarAction(id int) {
	switch id {
	case actionMonthBack:
		pickerMonth = pickerMonth.AddDate(0, -1, 0)
	case actionMonthNext:
		pickerMonth = pickerMonth.AddDate(0, 1, 0)
	case actionCalendarCancel:
		showCalendar(false)
		return
	default:
		index := id - actionCalendarDay
		if index < 0 || index >= 42 {
			return
		}
		day := monthGrid(pickerMonth)[index]
		if day.Month() != pickerMonth.Month() || localDate(day) < localDate(time.Now()) {
			return
		}
		setInput(fieldDate, localDate(day))
		showCalendar(false)
		return
	}
	updateCalendar()
}
