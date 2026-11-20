package main

import (
	"crypto/rand"
	"embed"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io"
	"maps"
	random "math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var taipei = time.FixedZone("Taipei", 8*3600)
var weekdays = [7]string{"日", "一", "二", "三", "四", "五", "六"}
var weekdayOrder = []int{1, 2, 3, 4, 5, 6, 0}

//go:embed art *.csv quotes/nonsense.txt
var resources embed.FS

type rule struct {
	ID, Title, Date, Time string
	Days                  []int
	Starts                time.Time
}

type item struct {
	Rule, Day, Title, Time string
	Completed              bool
}

type state struct {
	Rules   []rule
	Items   []item
	Scanned time.Time
	Cleared map[string]bool
}

type app struct {
	data      state
	path      string
	rulesPath string
}

func randomID() string {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw)
}

func localDate(now time.Time) string { return now.In(taipei).Format("2006-01-02") }

func advance(s state, now time.Time) (state, bool) {
	now = now.In(taipei)
	since := s.Scanned.In(taipei)
	if since.IsZero() || since.After(now) {
		since = now
	}
	midnight := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, taipei)
	s.Items = slices.Clone(s.Items)
	keys := make(map[string]bool, len(s.Items))
	for _, entry := range s.Items {
		keys[entry.Rule+"/"+entry.Day] = true
	}
	added := false
	for day := midnight; !day.After(now); day = day.AddDate(0, 0, 1) {
		date := localDate(day)
		for _, task := range s.Rules {
			if task.Date != "" && task.Date != date {
				continue
			}
			if task.Date == "" && !slices.Contains(task.Days, int(day.Weekday())) {
				continue
			}
			threshold, err := time.ParseInLocation("2006-01-02 15:04", date+" "+task.Time, taipei)
			if err != nil || threshold.After(now) || threshold.Before(task.Starts) || keys[task.ID+"/"+date] || s.Cleared[task.ID+"/"+date] {
				continue
			}
			s.Items = append(s.Items, item{Rule: task.ID, Day: date, Title: task.Title, Time: task.Time})
			keys[task.ID+"/"+date] = true
			added = true
		}
	}
	if now.Before(s.Scanned) {
		return s, added
	}
	s.Scanned = now
	return s, added
}

type clockState struct {
	Times        [2]string
	Minutes      int
	Pending      []string
	Acknowledged map[string]bool
	Days         map[string]bool
	Updated      string
	Hidden       bool
	QuietClock   bool
	Mascot       string
	SpriteScale  int
}

type row struct{ label, action, id, day string }

var (
	checklist                             app
	clock                                 clockState
	clockPath, settingsPath, calendarPath string
	rows                                  []row
	editing                               string
	showHistory                           bool
	nextSpeech, nextSync                  time.Time
	lastQuote                             = -1
	syncActive                            bool
	syncFailure                           bool
	syncResults                           = make(chan calendarReply, 1)
)

func scheduleText(task rule) string {
	if task.Date != "" {
		return task.Date + "  " + task.Time
	}
	var names []string
	for _, day := range weekdayOrder {
		if slices.Contains(task.Days, day) {
			names = append(names, weekdays[day])
		}
	}
	return "每週" + strings.Join(names, "、") + "  " + task.Time
}

func populate() {
	before := currentRow()
	var pending, history []row
	add := func(entry row) { pending = append(pending, entry) }
	for _, key := range clock.Pending {
		day, kind, _ := strings.Cut(key, "/")
		line := "上班打卡"
		if kind == "1" {
			line = "下班打卡"
		}
		add(row{"待打卡  " + day + "  " + line, "clock", key, ""})
	}
	for _, entry := range checklist.data.Items {
		if !entry.Completed {
			add(row{"□  " + entry.Day + " " + entry.Time + "  " + entry.Title, "complete", entry.Rule, entry.Day})
		}
	}
	for _, task := range checklist.data.Rules {
		add(row{"排程  " + scheduleText(task) + "  " + task.Title, "edit", task.ID, ""})
	}
	for _, entry := range slices.Backward(checklist.data.Items) {
		if entry.Completed {
			history = append(history, row{"✓  " + entry.Day + " " + entry.Time + "  " + entry.Title, "restore", entry.Rule, entry.Day})
		}
	}
	rows = pending
	if showHistory {
		rows = history
	}
	values := make([]string, len(rows))
	for i, entry := range rows {
		values[i] = entry.label
	}
	setHistoryView(showHistory)
	selection := 0
	if before.action != "" {
		for i, entry := range rows {
			if entry.id == before.id && entry.day == before.day && entry.action == before.action {
				selection = i
				break
			}
		}
	}
	setRows(values, selection)
	refreshReminder(pending)
}

func newRule() {
	editing = ""
	setInput(fieldTitle, "")
	setInput(fieldDate, localDate(time.Now()))
	setInput(fieldTime, "09:00")
	setMode(false)
	for i := range weekdays {
		setChecked(i, false)
	}
}

func currentRow() row {
	index := selected()
	if index >= 0 && index < len(rows) {
		return rows[index]
	}
	return row{}
}
func onAction(id int) {
	switch id {
	case actionQuoteSettings, actionQuoteSave, actionQuoteImport, actionQuoteExport:
		handleQuoteAction(id)
	case actionAppearance:
		showAppearance()
	case actionClear:
		newRule()
		setInput(fieldDate, "")
		setInput(fieldTime, "")
	case actionPickDate:
		openCalendar()
	case actionAutostart:
		if err := setAutostart(autostartChecked()); err != nil {
			notice(err.Error())
		}
		refreshAutostart()
	case actionMonthBack, actionMonthNext, actionCalendarCancel:
		calendarAction(id)
	case actionMode:
		modeChanged()
	case actionClock:
		next := clock
		for i, field := range []int{fieldIn, fieldOut} {
			next.Times[i] = strings.TrimSpace(input(field))
			if !validTime(next.Times[i]) {
				notice("請輸入 24 小時制時間，例如 08:30。")
				return
			}
		}
		minutes, err := strconv.Atoi(strings.TrimSpace(input(fieldSpeech)))
		if err != nil || minutes < 1 || minutes > 1440 {
			notice("說話間隔請輸入 1 到 1440 分鐘。")
			return
		}
		next.Minutes = minutes
		next.QuietClock = !forceClockChecked()
		if err = saveClockSettings(next); err != nil {
			notice(err.Error())
			return
		}
		nextSpeech = time.Now().Add(time.Duration(minutes) * time.Minute)
		populate()
		tick()
	case actionSave:
		if err := saveRule(time.Now()); err != nil {
			notice(err.Error())
			return
		}
		newRule()
		populate()
	case actionEdit:
		selected := currentRow()
		if selected.action != "edit" {
			notice("請先選取一筆排程。")
			return
		}
		for _, task := range checklist.data.Rules {
			if task.ID != selected.id {
				continue
			}
			editing = task.ID
			setInput(fieldTitle, task.Title)
			setInput(fieldDate, task.Date)
			setInput(fieldTime, task.Time)
			if task.Date == "" {
				setInput(fieldDate, localDate(time.Now()))
			}
			for i := range weekdays {
				setChecked(i, slices.Contains(task.Days, i))
			}
			setMode(task.Date == "")
		}
	case actionDelete:
		selected := currentRow()
		if selected.action != "edit" {
			notice("請先選取一筆排程。")
			return
		}
		next := checklist.data
		next.Rules = slices.DeleteFunc(slices.Clone(next.Rules), func(task rule) bool { return task.ID == selected.id })
		next.Items = slices.DeleteFunc(slices.Clone(next.Items), func(entry item) bool { return entry.Rule == selected.id && !entry.Completed })
		if err := checklist.saveRules(next); err != nil {
			notice(err.Error())
			return
		}
		if editing == selected.id {
			newRule()
		}
		populate()
	case actionDone:
		completeRow(currentRow())
	case actionClearHistory:
		if !showHistory || len(rows) == 0 || !confirmClearHistory() {
			return
		}
		if err := checklist.commit(clearCompleted(checklist.data)); err != nil {
			notice(err.Error())
			return
		}
		populate()
	case actionShowPending, actionShowHistory:
		showHistory = id == actionShowHistory
		populate()
	case actionConfirmReminder:
		if activeReminder.action != "" {
			completeRow(activeReminder)
		}
	default:
		if id >= actionCalendarDay && id < actionCalendarDay+42 {
			calendarAction(id)
		}
	}
}

func validTime(raw string) bool {
	matched, err := time.Parse("15:04", raw)
	return err == nil && matched.Format("15:04") == raw
}

func saveRule(now time.Time) error {
	task := rule{ID: editing, Title: strings.TrimSpace(input(fieldTitle)), Time: strings.TrimSpace(input(fieldTime)), Starts: now.In(taipei).Truncate(time.Minute)}
	if n := len([]rune(task.Title)); n < 1 || n > 100 {
		return errors.New("請輸入 1 到 100 字的提醒名稱。")
	}
	if !validTime(task.Time) {
		return errors.New("請輸入 24 小時制時間，例如 09:30。")
	}
	var dateTime time.Time
	if weeklyMode() {
		for i := range weekdays {
			if checked(i) {
				task.Days = append(task.Days, i)
			}
		}
		if len(task.Days) == 0 {
			return errors.New("請選擇至少一個星期。")
		}
	} else {
		task.Date = strings.TrimSpace(input(fieldDate))
		var err error
		dateTime, err = time.ParseInLocation("2006-01-02 15:04", task.Date+" "+task.Time, taipei)
		if err != nil {
			return errors.New("請輸入有效日期，例如 2026-11-30。")
		}
	}
	next := checklist.data
	next.Rules = slices.Clone(next.Rules)
	index := slices.IndexFunc(next.Rules, func(prior rule) bool { return prior.ID == task.ID })
	if index >= 0 {
		old := next.Rules[index]
		if old.Date == task.Date && old.Time == task.Time && slices.Equal(old.Days, task.Days) {
			task.Starts = old.Starts
		}
	}
	if task.Date != "" && dateTime.Before(task.Starts) {
		return errors.New("請選擇現在或未來的日期與時間。")
	}
	if index < 0 {
		task.ID = randomID()
		next.Rules = append(next.Rules, task)
	} else {
		next.Rules[index] = task
	}
	return checklist.saveRules(next)
}

func openFields() {
	setAppearanceLabel(spriteLabel())
	setForceClockChecked(!clock.QuietClock)
	setInput(fieldIn, clock.Times[0])
	setInput(fieldOut, clock.Times[1])
	setInput(fieldSpeech, strconv.Itoa(clock.Minutes))
	populate()
}
func openSettings() { openFields(); refreshAutostart(); showSettings() }
func hideMascot(hidden bool) {
	next := clock
	next.Hidden = hidden
	if err := saveClockSettings(next); err != nil {
		notice(err.Error())
		return
	}
	if activeReminder.action != "" {
		showMascot(true)
		return
	}
	if !hidden && lastQuote < 0 {
		speak(time.Now())
	}
	showMascot(!hidden)
}
func speakNow() { speak(time.Now()) }
func speak(now time.Time) {
	if clock.Hidden || activeReminder.action != "" {
		return
	}
	quoteCount := speechCount()
	var pick int
	if lastQuote >= 0 && lastQuote < quoteCount {
		pick = random.IntN(quoteCount - 1)
		if pick >= lastQuote {
			pick++
		}
	} else {
		pick = random.IntN(quoteCount)
	}
	lastQuote = pick
	paintMascot(pick)
	nextSpeech = now.Add(time.Duration(clock.Minutes) * time.Minute)
}
func shouldSyncCalendar(now time.Time, updated string, hasDate, active bool, retry time.Time) bool {
	now = now.In(taipei)
	return !active && !now.Before(retry) && (!hasDate || (now.Hour() >= 9 && updated != localDate(now)))
}

func tick() {
	now := time.Now().In(taipei)
	today := localDate(now)
	select {
	case reply := <-syncResults:
		syncActive = false
		syncFailure = reply.err != nil
		if !syncFailure {
			next := clock
			next.Days = maps.Clone(clock.Days)
			for k, v := range reply.days {
				next.Days[k] = v
			}
			if now.Hour() >= 9 {
				next.Updated = today
			}
			syncFailure = saveCalendarCache(next) != nil
		}
		if syncFailure {
			nextSync = now.Add(15 * time.Minute)
		}
	default:
	}
	_, hasDate := clock.Days[today]
	if shouldSyncCalendar(now, clock.Updated, hasDate, syncActive, nextSync) {
		syncActive = true
		go refreshCalendar(now.Year())
	}
	status := "日曆最近更新 " + clock.Updated
	if clock.Updated == "" {
		status = "辦公日曆每日 09:00 更新"
	}
	if syncFailure {
		status = "辦公日曆更新失敗，稍後重試"
	}
	if syncActive {
		status = "正在更新辦公日曆"
	}
	setStatus(status)
	pendingClock := false
	if clock.Days[today] {
		next := clock
		next.Pending = slices.Clone(next.Pending)
		for i, limit := range clock.Times {
			key := today + "/" + strconv.Itoa(i)
			if now.Format("15:04") >= limit && !clock.Acknowledged[key] && !slices.Contains(next.Pending, key) {
				next.Pending = append(next.Pending, key)
			}
		}
		if len(next.Pending) > len(clock.Pending) {
			pendingClock = writeClock(next) == nil
		}
	}
	next, added := advance(checklist.data, now)
	if added || now.Sub(checklist.data.Scanned) >= time.Minute {
		if checklist.commit(next) != nil {
			added = false
		}
	}
	if added || pendingClock {
		populate()
	}
	if !now.Before(nextSpeech) {
		speak(now)
	}
}

type calendarReply struct {
	days map[string]bool
	err  error
}

var csvClient = &http.Client{Timeout: 12 * time.Second}
var csvLink = regexp.MustCompile(`(?i)https://www\.dgpa\.gov\.tw/FileConversion\?[^"'<>\s\\]+`)
var csvName = regexp.MustCompile(`^(\d{3})年.*辦公日曆表.*\.csv$`)

func fetch(address string) ([]byte, error) {
	response, err := csvClient.Get(address)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 8<<20))
}
func calendarLinks(page string) map[int]string {
	page = strings.NewReplacer(`\u0026`, "&", `\u002F`, "/", `\/`, "/").Replace(html.UnescapeString(page))
	links := map[int]string{}
	for _, link := range csvLink.FindAllString(page, -1) {
		parsed, err := url.Parse(link)
		if err != nil {
			continue
		}
		name := parsed.Query().Get("name")
		match := csvName.FindStringSubmatch(name)
		if match == nil || strings.Contains(strings.ToLower(name), "google") {
			continue
		}
		y, _ := strconv.Atoi(match[1])
		if links[y+1911] == "" || strings.Contains(name, "更新") || strings.Contains(name, "修正") {
			links[y+1911] = link
		}
	}
	return links
}
func parseCalendar(raw []byte, year int) (map[string]bool, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff")))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	dateColumn, holidayColumn := -1, -1
	for i, value := range header {
		switch strings.TrimSpace(value) {
		case "西元日期", "\xa6\xe8\xa4\xb8\xa4\xe9\xb4\xc1":
			dateColumn = i
		case "是否放假", "\xac\x4f\xa7\x5f\xa9\xf1\xb0\xb2":
			holidayColumn = i
		}
	}
	if dateColumn < 0 || holidayColumn < 0 {
		return nil, errors.New("欄位錯誤")
	}
	days := map[string]bool{}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) <= max(dateColumn, holidayColumn) {
			return nil, errors.New("欄位不足")
		}
		raw := strings.TrimSpace(row[dateColumn])
		parsed, err := time.Parse("20060102", raw)
		if err != nil {
			parsed, err = time.Parse("2006/1/2", raw)
		}
		if err != nil {
			parsed, err = time.Parse("2006-01-02", raw)
		}
		if err != nil || parsed.Year() != year {
			return nil, errors.New("日期錯誤")
		}
		value := strings.TrimSpace(row[holidayColumn])
		if value != "0" && value != "2" {
			return nil, errors.New("假日欄位錯誤")
		}
		days[parsed.Format("2006-01-02")] = value == "0"
	}
	if len(days) != time.Date(year, 12, 31, 0, 0, 0, 0, taipei).YearDay() {
		return nil, errors.New("日曆不完整")
	}
	return days, nil
}
func refreshCalendar(year int) {
	body, err := fetch("https://data.gov.tw/dataset/14718")
	if err != nil {
		syncResults <- calendarReply{err: err}
		return
	}
	links := calendarLinks(string(body))
	days := map[string]bool{}
	for i := year; i <= year+1; i++ {
		if links[i] == "" {
			if i == year {
				err = errors.New("找不到日曆")
			}
			continue
		}
		raw, e := fetch(links[i])
		var parsed map[string]bool
		if e == nil {
			parsed, e = parseCalendar(raw, i)
		}
		if e != nil && i == year {
			err = e
		}
		for k, v := range parsed {
			days[k] = v
		}
	}
	syncResults <- calendarReply{days, err}
}

const (
	fieldIn = 100 + iota
	fieldOut
	fieldSpeech
	fieldTitle
	fieldDate
	fieldTime
	actionClock
	actionClear
	actionSave
	actionEdit
	actionDelete
	actionDone
	actionMode
	actionPickDate
	actionAutostart
	actionMonthBack
	actionMonthNext
	actionCalendarCancel
	actionConfirmReminder
	actionShowPending
	actionShowHistory
	actionClearHistory
	actionAppearance
	actionQuoteSettings
	actionQuoteSave
	actionQuoteImport
	actionQuoteExport
	actionCalendarDay = 200
)
