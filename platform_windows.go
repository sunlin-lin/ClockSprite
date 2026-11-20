package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	imagedraw "image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var user = syscall.NewLazyDLL("user32.dll")
var shell = syscall.NewLazyDLL("shell32.dll")
var kernel = syscall.NewLazyDLL("kernel32.dll")
var tray notifyData
var mascotWindow, settingsWindow, instance, icon, list, status, mode uintptr
var pendingTab, historyTab, completeButton, editButton, deleteButton, clearHistoryButton, forceClockButton uintptr
var reminderWindow, reminderTitle, reminderDetail, reminderBrush uintptr
var dateButton, startupButton, calendarWindow, calendarTitle uintptr
var dateCells [42]uintptr
var fields = map[int]uintptr{}
var dayButtons [7]uintptr
var gdi = syscall.NewLazyDLL("gdi32.dll")
var recreate uintptr
var spriteVisible, spriteTimer bool
var spriteEpoch time.Time
var spriteFrame, bubblePick = -1, -1
var spriteBubble image.Image
var fallbackSprite image.Image
var appearanceButton, reminderConfirm uintptr
var currentScale = 100
var resizing bool
var resizeAnchor point
var initialScale int

type notifyData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	Title               [64]uint16
	InfoFlags           uint32
	Guid                [16]byte
	BalloonIcon         uintptr
}
type point struct{ X, Y int32 }
type message struct {
	Window         uintptr
	Code           uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type winClass struct {
	Size, Style                                          uint32
	Callback                                             uintptr
	ClassExtra, WindowExtra                              int32
	Instance, Icon, Cursor, Brush, Menu, Name, SmallIcon uintptr
}

func win(dll *syscall.LazyDLL, name string, args ...uintptr) uintptr {
	result, _, _ := dll.NewProc(name).Call(args...)
	return result
}
func wtext(s string) *uint16  { v, _ := syscall.UTF16PtrFromString(s); return v }
func ptr[T any](p *T) uintptr { return uintptr(unsafe.Pointer(p)) }

var errRunning = syscall.Errno(32)

func acquireLock(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func legacyClockPath(root string) string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		local = root
	}
	return filepath.Join(local, "ClockSprite", "native-state.json")
}
func notice(message string) {
	owner := settingsWindow
	if quoteWindow != 0 && win(user, "IsWindowVisible", quoteWindow) != 0 {
		owner = quoteWindow
	}
	win(user, "MessageBoxW", owner, ptr(wtext(message)), ptr(wtext("社畜小精靈")), 0x30)
}
func notify() {
	tray = notifyData{Window: mascotWindow, ID: 1, Flags: 7, Callback: 0x8001, Icon: icon}
	tray.Size = uint32(unsafe.Sizeof(tray))
	copy(tray.Tip[:], syscall.StringToUTF16("社畜小精靈"))
	win(shell, "Shell_NotifyIconW", 0, ptr(&tray))
}
func menu(window uintptr) {
	popup := win(user, "CreatePopupMenu")
	for i, label := range []string{"設定", "顯示小精靈", "隱藏小精靈", "結束社畜小精靈"} {
		win(user, "AppendMenuW", popup, 0, uintptr(i+1), ptr(wtext(label)))
	}
	var mouse point
	win(user, "GetCursorPos", ptr(&mouse))
	win(user, "SetForegroundWindow", window)
	choice := win(user, "TrackPopupMenu", popup, 0x102, uintptr(mouse.X), uintptr(mouse.Y), 0, window, 0)
	win(user, "DestroyMenu", popup)
	switch choice {
	case 1:
		openSettings()
	case 2:
		hideMascot(false)
	case 3:
		hideMascot(true)
	case 4:
		win(user, "DestroyWindow", mascotWindow)
	}
}
func wndProc(window, code, wp, lp uintptr) uintptr {
	switch code {
	case recreate:
		if window == mascotWindow {
			notify()
		}
	case 0x8001:
		if lp == 0x202 {
			hideMascot(false)
		}
		if lp == 0x205 {
			menu(window)
		}
		return 0
	case 0x8002:
		openSettings()
		return 0
	case 0x205:
		if window == mascotWindow {
			menu(window)
			return 0
		}
	case 0x200:
		if window == mascotWindow && resizing {
			var cursor point
			win(user, "GetCursorPos", ptr(&cursor))
			changeSpriteScale(dragScale(initialScale, int(cursor.X-resizeAnchor.X), int(cursor.Y-resizeAnchor.Y)))
			return 0
		}
	case 0x202:
		if window == mascotWindow && resizing {
			resizing = false
			win(user, "ReleaseCapture")
			savedSpriteScale(currentScale)
			return 0
		}
	case 0x215:
		if window == mascotWindow && resizing {
			resizing = false
			savedSpriteScale(currentScale)
			return 0
		}
	case 0x201:
		if window == mascotWindow {
			var pt point
			win(user, "GetCursorPos", ptr(&pt))
			x, y := int(int16(lp)), int(int16(lp>>16))
			if x >= scaled(313) && y >= scaled(280) {
				resizeAnchor = pt
				initialScale = currentScale
				resizing = true
				win(user, "SetCapture", mascotWindow)
				return 0
			}
			win(user, "ReleaseCapture")
			win(user, "SendMessageW", window, 0xA1, 2, uintptr(uint16(pt.X))|uintptr(uint16(pt.Y))<<16)
			return 0
		}
	case 0x47:
		if window == mascotWindow && activeReminder.action != "" {
			positionReminder()
		}
	case 0x138:
		if window == reminderWindow {
			win(gdi, "SetBkColor", wp, reminderColor)
			return reminderBrush
		}
	case 0x14:
		if window == reminderWindow {
			var area [4]int32
			win(user, "GetClientRect", window, ptr(&area))
			win(user, "FillRect", wp, ptr(&area), reminderBrush)
			return 1
		}
	case 0x111:
		if (window == settingsWindow || window == calendarWindow || window == reminderWindow || window == quoteWindow) && wp>>16 == 0 {
			onAction(int(wp & 0xffff))
			return 0
		}
	case 0x113:
		if window == mascotWindow {
			if wp == 2 {
				animateSprite()
			} else if wp == 1 {
				tick()
			}
			return 0
		}
	case 0x10:
		if window == quoteWindow {
			closeQuoteEditor()
			return 0
		}
		if window == calendarWindow {
			showCalendar(false)
			return 0
		}
		if window == settingsWindow {
			showCalendar(false)
			win(user, "ShowWindow", window, 0)
			return 0
		}
	case 2:
		if window == mascotWindow {
			win(shell, "Shell_NotifyIconW", 2, ptr(&tray))
			win(user, "PostQuitMessage", 0)
			return 0
		}
	}
	return win(user, "DefWindowProcW", window, code, wp, lp)
}

func child(kind, title string, style, x, y, w, h, id uintptr) uintptr {
	return makeChild(settingsWindow, kind, title, style, x, y, w, h, id)
}
func makeChild(parent uintptr, kind, title string, style, x, y, w, h, id uintptr) uintptr {
	hWnd := win(user, "CreateWindowExW", 0, ptr(wtext(kind)), ptr(wtext(title)), 0x50000000|style, x, y, w, h, parent, id, instance, 0)
	font := win(gdi, "GetStockObject", 17)
	win(user, "SendMessageW", hWnd, 0x30, font, 1)
	return hWnd
}
func labelW(title string, x, y, w uintptr) { child("STATIC", title, 0, x, y, w, 20, 0) }
func editW(id int, x, y, w uintptr) {
	fields[id] = child("EDIT", "", 0x810080, x, y, w, 25, uintptr(id))
}
func buttonW(title string, id int, x, y, w uintptr) {
	child("BUTTON", title, 0x10000, x, y, w, 27, uintptr(id))
}
func buildSettings() {
	buttonW("幹話語錄", actionQuoteSettings, 18, 9, 137)
	forceClockButton = child("BUTTON", "強制提醒上下班打卡", 0x10003, 190, 9, 235, 25, 0)
	appearanceButton = child("BUTTON", "角色圖示", 0x10000, 440, 9, 163, 26, actionAppearance)
	labelW("上班", 18, 43, 40)
	editW(fieldIn, 62, 39, 70)
	labelW("下班", 145, 43, 40)
	editW(fieldOut, 190, 39, 70)
	labelW("語錄間隔（分鐘）", 272, 43, 130)
	editW(fieldSpeech, 402, 39, 58)
	buttonW("儲存打卡設定", actionClock, 472, 38, 132)
	status = child("STATIC", "", 0, 18, 73, 350, 22, 0)
	startupButton = child("BUTTON", "登入電腦時自動啟動", 0x10003, 381, 70, 223, 26, actionAutostart)
	labelW("自訂提醒", 18, 106, 120)
	labelW("提醒名稱", 18, 137, 70)
	editW(fieldTitle, 90, 133, 514)
	mode = child("BUTTON", "每週固定", 0x10003, 18, 167, 90, 25, actionMode)
	labelW("日期", 118, 171, 36)
	fields[fieldDate] = child("EDIT", "", 0x810880, 158, 165, 118, 25, fieldDate)
	dateButton = child("BUTTON", "選日期", 0x10000, 284, 165, 91, 26, actionPickDate)
	labelW("時間", 397, 171, 40)
	editW(fieldTime, 445, 165, 78)
	for i, day := range weekdayOrder {
		dayButtons[day] = child("BUTTON", "週"+weekdays[day], 0x10003, 18+uintptr(i)*68, 196, 65, 26, 0)
	}
	buttonW("清空", actionClear, 18, 231, 105)
	buttonW("儲存提醒", actionSave, 132, 231, 104)
	repeatButton = child("BUTTON", "時段內重複", 0x10003, 249, 231, 104, 26, actionRepeatMode)
	labelW("到", 360, 236, 20)
	editW(fieldEndTime, 382, 231, 69)
	labelW("每隔", 460, 236, 34)
	editW(fieldInterval, 499, 231, 48)
	labelW("分鐘", 554, 236, 46)
	pendingTab = child("BUTTON", "待辦與排程", 0x30009, 18, 269, 108, 25, actionShowPending)
	historyTab = child("BUTTON", "完成紀錄", 0x10009, 136, 269, 105, 25, actionShowHistory)
	list = child("LISTBOX", "", 0xA10001, 18, 298, 586, 190, 0)
	completeButton = child("BUTTON", "勾選完成", 0x10000, 18, 500, 137, 27, actionDone)
	editButton = child("BUTTON", "編輯排程", 0x10000, 164, 500, 104, 27, actionEdit)
	deleteButton = child("BUTTON", "刪除排程", 0x10000, 277, 500, 104, 27, actionDelete)
	clearHistoryButton = child("BUTTON", "清空紀錄", 0x10000, 164, 500, 104, 27, actionClearHistory)
}
func setInput(id int, value string) { win(user, "SetWindowTextW", fields[id], ptr(wtext(value))) }
func input(id int) string {
	var raw [256]uint16
	win(user, "GetWindowTextW", fields[id], ptr(&raw), 256)
	return syscall.UTF16ToString(raw[:])
}
func setChecked(day int, on bool) { win(user, "SendMessageW", dayButtons[day], 0xF1, boolW(on), 0) }
func checked(day int) bool        { return win(user, "SendMessageW", dayButtons[day], 0xF0, 0, 0) == 1 }
func boolW(value bool) uintptr {
	if value {
		return 1
	}
	return 0
}
func setMode(on bool)  { win(user, "SendMessageW", mode, 0xF1, boolW(on), 0); modeChanged() }
func weeklyMode() bool { return win(user, "SendMessageW", mode, 0xF0, 0, 0) == 1 }
func modeChanged() {
	active := weeklyMode()
	win(user, "EnableWindow", fields[fieldDate], boolW(!active))
	win(user, "EnableWindow", dateButton, boolW(!active))
	if active {
		showCalendar(false)
	}
	for _, item := range dayButtons {
		win(user, "EnableWindow", item, boolW(active))
	}
}
func setRows(values []string, index int) {
	win(user, "SendMessageW", list, 0x184, 0, 0)
	for _, value := range values {
		win(user, "SendMessageW", list, 0x180, 0, ptr(wtext(value)))
	}
	if len(values) > 0 {
		win(user, "SendMessageW", list, 0x186, uintptr(max(0, min(index, len(values)-1))), 0)
	}
}
func selected() int          { return int(win(user, "SendMessageW", list, 0x188, 0, 0)) }
func setStatus(value string) { win(user, "SetWindowTextW", status, ptr(wtext(value))) }
func showSettings() {
	win(user, "ShowWindow", settingsWindow, 5)
	win(user, "SetForegroundWindow", settingsWindow)
}
func showMascot(on bool) {
	command := uintptr(0)
	if on {
		command = 4
	}
	spriteVisible = on
	updateSpriteTimer()
	win(user, "ShowWindow", mascotWindow, command)
	if customSpeech(bubblePick) != "" {
		showQuoteBubbleW()
	}
}
func updateSpriteTimer() {
	play := spriteVisible && activeReminder.action == "" && len(activeSprite().frames) > 1
	if play == spriteTimer {
		return
	}
	spriteTimer = play
	if play {
		spriteEpoch = time.Now()
		win(user, "SetTimer", mascotWindow, 2, 25, 0)
	} else {
		win(user, "KillTimer", mascotWindow, 2)
	}
}
func animateSprite() {
	if !spriteTimer {
		return
	}
	next := activeSprite().index(time.Since(spriteEpoch))
	if next == spriteFrame {
		return
	}
	spriteFrame = next
	drawMascot()
}
func paintMascot(pick int) {
	if activeReminder.action != "" || spriteFrame < 0 {
		spriteFrame = activeSprite().index(0)
	}
	wasQuote := customSpeech(bubblePick) != "" || quoteOverlay
	if pick != bubblePick {
		bubblePick = pick
		spriteBubble = nil
		if pick >= 0 || pick == -2 {
			path := "art/reminder_cloud.png"
			if pick >= 0 && customSpeech(pick) == "" {
				path = fmt.Sprintf("art/bubbles/%03d.png", pick)
			}
			raw, _ := resources.ReadFile(path)
			spriteBubble, _ = png.Decode(bytes.NewReader(raw))
		}
	}
	updateSpriteTimer()
	drawMascot()
	if customSpeech(pick) != "" {
		showQuoteBubbleW()
	} else if wasQuote {
		quoteOverlay = false
		hideReminderBubble()
	}
}
func drawMascot() {
	frame := image.NewRGBA(image.Rect(0, 0, mascotWidth, mascotHeight))
	animation := activeSprite()
	if spriteFrame >= 0 && spriteFrame < len(animation.frames) {
		sprite := animation.frames[spriteFrame]
		imagedraw.Draw(frame, sprite.Bounds().Add(image.Pt(spriteX, spriteY)), sprite, sprite.Bounds().Min, imagedraw.Over)
	} else {
		if fallbackSprite == nil {
			raw, _ := resources.ReadFile("art/floating_mascot.png")
			if decoded, err := png.Decode(bytes.NewReader(raw)); err == nil {
				fallbackSprite = placeMascot(decoded)
			}
		}
		if fallbackSprite != nil {
			imagedraw.Draw(frame, fallbackSprite.Bounds().Add(image.Pt(spriteX, spriteY)), fallbackSprite, fallbackSprite.Bounds().Min, imagedraw.Over)
		}
	}
	if spriteBubble != nil {
		imagedraw.Draw(frame, spriteBubble.Bounds(), spriteBubble, spriteBubble.Bounds().Min, imagedraw.Over)
	}
	drawGrip(frame)
	width, height := scaled(mascotWidth), scaled(mascotHeight)
	if currentScale != 100 {
		frame = resizeRGBA(frame, width, height)
	}
	dc := win(gdi, "CreateCompatibleDC", 0)
	if dc == 0 {
		return
	}
	defer win(gdi, "DeleteDC", dc)
	var info [40]byte
	binary.LittleEndian.PutUint32(info[:], 40)
	binary.LittleEndian.PutUint32(info[4:], uint32(width))
	negativeHeight := int32(-height)
	binary.LittleEndian.PutUint32(info[8:], uint32(negativeHeight))
	binary.LittleEndian.PutUint16(info[12:], 1)
	binary.LittleEndian.PutUint16(info[14:], 32)
	var bits unsafe.Pointer
	bitmap := win(gdi, "CreateDIBSection", dc, ptr(&info), 0, ptr(&bits), 0, 0)
	runtime.KeepAlive(&info)
	if bitmap == 0 || bits == nil {
		return
	}
	defer win(gdi, "DeleteObject", bitmap)
	old := win(gdi, "SelectObject", dc, bitmap)
	defer win(gdi, "SelectObject", dc, old)
	pixels := unsafe.Slice((*byte)(bits), len(frame.Pix))
	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = frame.Pix[i+2], frame.Pix[i+1], frame.Pix[i], frame.Pix[i+3]
	}
	size, origin, blend := point{int32(width), int32(height)}, point{}, [4]byte{0, 0, 255, 1}
	win(user, "UpdateLayeredWindow", mascotWindow, 0, 0, ptr(&size), dc, ptr(&origin), 0, ptr(&blend), 2)
	runtime.KeepAlive(&size)
	runtime.KeepAlive(&origin)
	runtime.KeepAlive(&blend)
}
func main() {
	runtime.LockOSThread()
	lock, err := load()
	if err != nil {
		if err == errRunning {
			old := win(user, "FindWindowW", ptr(wtext("ClockSpriteUnified")), 0)
			if old != 0 {
				win(user, "PostMessageW", old, 0x8002, 0, 0)
				return
			}
			notice("請先結束舊版提醒清單，再開啟新版小精靈。")
			return
		}
		notice(err.Error())
		return
	}
	defer lock.Close()
	instance = win(kernel, "GetModuleHandleW", 0)
	raw, _ := resources.ReadFile("art/tray_mascot.png")
	icon = win(user, "CreateIconFromResourceEx", ptr(&raw[0]), uintptr(len(raw)), 1, 0x30000, 32, 32, 0)
	runtime.KeepAlive(raw)
	if icon == 0 {
		icon = win(user, "LoadIconW", 0, 32512)
	}
	name := wtext("ClockSpriteUnified")
	cursor := win(user, "LoadCursorW", 0, 32512)
	wc := winClass{Instance: instance, Name: ptr(name), Cursor: cursor, Icon: icon, SmallIcon: icon, Brush: 6, Callback: syscall.NewCallback(wndProc)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	recreate = win(user, "RegisterWindowMessageW", ptr(wtext("TaskbarCreated")))
	win(user, "RegisterClassExW", ptr(&wc))
	var area [4]int32
	win(user, "SystemParametersInfoW", 48, 0, ptr(&area), 0)
	mascotWindow = win(user, "CreateWindowExW", 0x80088, ptr(name), ptr(wtext("社畜小精靈")), 0x80000000, uintptr(max(0, area[2]-364)), uintptr(max(0, area[3]-334)), 340, 310, 0, 0, instance, 0)
	settingsWindow = win(user, "CreateWindowExW", 0x81, ptr(name), ptr(wtext("社畜小精靈設定")), 0x80CA0000, uintptr(max(0, (area[2]-640)/2)), uintptr(max(0, (area[3]-575)/2)), 640, 575, 0, 0, instance, 0)
	if mascotWindow == 0 || settingsWindow == 0 {
		notice("無法建立視窗。")
		return
	}
	buildSettings()
	currentScale = 100
	calendarWindow = win(user, "CreateWindowExW", 0x80, ptr(name), ptr(wtext("選擇日期")), 0x80C80000, 0, 0, 316, 334, settingsWindow, 0, instance, 0)
	if calendarWindow == 0 {
		notice("無法建立月曆視窗。")
		return
	}
	buildCalendar()
	reminderWindow = win(user, "CreateWindowExW", 0x88, ptr(name), ptr(wtext("小精靈提醒")), 0x80000000, 0, 0, 244, 80, mascotWindow, 0, instance, 0)
	if reminderWindow == 0 {
		notice("無法建立提醒視窗。")
		return
	}
	defer win(gdi, "DeleteObject", buildReminder())
	changeSpriteScale(storedSpriteScale())
	newRule()
	openFields()
	notify()
	if !clock.Hidden && activeReminder.action == "" {
		speakNow()
		showMascot(true)
	}
	tick()
	win(user, "SetTimer", mascotWindow, 1, 2000, 0)
	var msg message
	for int32(win(user, "GetMessageW", ptr(&msg), 0, 0, 0)) > 0 {
		if win(user, "IsWindowVisible", calendarWindow) != 0 && win(user, "IsDialogMessageW", calendarWindow, ptr(&msg)) != 0 {
			continue
		}
		if win(user, "IsWindowVisible", reminderWindow) != 0 && win(user, "IsDialogMessageW", reminderWindow, ptr(&msg)) != 0 {
			continue
		}
		if win(user, "IsWindowVisible", quoteWindow) != 0 && win(user, "IsDialogMessageW", quoteWindow, ptr(&msg)) != 0 {
			continue
		}
		if win(user, "IsDialogMessageW", settingsWindow, ptr(&msg)) != 0 {
			continue
		}
		win(user, "TranslateMessage", ptr(&msg))
		win(user, "DispatchMessageW", ptr(&msg))
	}
}

func buildCalendar() {
	const button = 0x10000
	makeChild(calendarWindow, "BUTTON", "上個月", button, 14, 12, 70, 26, actionMonthBack)
	calendarTitle = makeChild(calendarWindow, "STATIC", "", 1, 90, 17, 120, 24, 0)
	makeChild(calendarWindow, "BUTTON", "下個月", button, 217, 12, 70, 26, actionMonthNext)
	for index, name := range weekdays {
		makeChild(calendarWindow, "STATIC", name, 1, 14+40*uintptr(index), 49, 35, 21, 0)
	}
	for i := range dateCells {
		dateCells[i] = makeChild(calendarWindow, "BUTTON", "", button, 14+40*uintptr(i%7), 73+30*uintptr(i/7), 35, 26, uintptr(actionCalendarDay+i))
	}
	makeChild(calendarWindow, "BUTTON", "取消", button, 217, 262, 70, 26, actionCalendarCancel)
}
func renderCalendar(title string, labels [42]string, enabled [42]bool) {
	win(user, "SetWindowTextW", calendarTitle, ptr(wtext(title)))
	for i, handle := range dateCells {
		win(user, "SetWindowTextW", handle, ptr(wtext(labels[i])))
		win(user, "EnableWindow", handle, boolW(enabled[i]))
		win(user, "ShowWindow", handle, 5*boolW(labels[i] != ""))
	}
}
func showCalendar(open bool) {
	if calendarWindow == 0 {
		return
	}
	if !open {
		if win(user, "IsWindowVisible", calendarWindow) != 0 {
			win(user, "ShowWindow", calendarWindow, 0)
			win(user, "SetForegroundWindow", settingsWindow)
		}
		return
	}
	var area [4]int32
	win(user, "GetWindowRect", settingsWindow, ptr(&area))
	x := area[0] + (area[2]-area[0]-316)/2
	y := area[1] + (area[3]-area[1]-334)/2
	win(user, "SetWindowPos", calendarWindow, 0, uintptr(x), uintptr(y), 0, 0, 0x41)
	win(user, "SetForegroundWindow", calendarWindow)
}

const reminderColor = 0xEEF9FF

func buildReminder() uintptr {
	reminderBrush = win(gdi, "CreateSolidBrush", reminderColor)
	reminderTitle = makeChild(reminderWindow, "STATIC", "", 1|0x80, 0, 2, 244, 32, 0)
	quoteBubbleText = makeChild(reminderWindow, "STATIC", "", 1|0x80, 0, 5, 244, 70, 0)
	win(user, "ShowWindow", quoteBubbleText, 0)
	reminderDetail = makeChild(reminderWindow, "STATIC", "", 1, 0, 34, 244, 17, 0)
	reminderConfirm = makeChild(reminderWindow, "BUTTON", "確認", 0x10000, 76, 52, 92, 25, actionConfirmReminder)
	return reminderBrush
}
func positionReminder() {
	if reminderWindow == 0 {
		return
	}
	var area [4]int32
	win(user, "GetWindowRect", mascotWindow, ptr(&area))
	win(user, "SetWindowPos", reminderWindow, 0, uintptr(int(area[0])+scaled(48)), uintptr(int(area[1])+scaled(23)), uintptr(scaled(244)), uintptr(scaled(80)), 0x14)
}
func showReminderBubble(title, detail string) {
	paintMascot(-2)
	win(user, "ShowWindow", quoteBubbleText, 0)
	for _, control := range []uintptr{reminderTitle, reminderDetail, reminderConfirm} {
		win(user, "ShowWindow", control, 5)
	}
	win(user, "SetWindowTextW", reminderTitle, ptr(wtext(title)))
	win(user, "SetWindowTextW", reminderDetail, ptr(wtext(detail)))
	positionReminder()
	win(user, "ShowWindow", reminderWindow, 4)
}
func hideReminderBubble() { win(user, "ShowWindow", reminderWindow, 0) }

func setHistoryView(history bool) {
	win(user, "SendMessageW", pendingTab, 0xF1, boolW(!history), 0)
	win(user, "SendMessageW", historyTab, 0xF1, boolW(history), 0)
	title := "勾選完成"
	if history {
		title = "取消完成"
	}
	win(user, "SetWindowTextW", completeButton, ptr(wtext(title)))
	win(user, "ShowWindow", editButton, 5*boolW(!history))
	win(user, "ShowWindow", deleteButton, 5*boolW(!history))
	win(user, "ShowWindow", clearHistoryButton, 5*boolW(history))
}

func forceClockChecked() bool      { return win(user, "SendMessageW", forceClockButton, 0xF0, 0, 0) == 1 }
func setForceClockChecked(on bool) { win(user, "SendMessageW", forceClockButton, 0xF1, boolW(on), 0) }
func confirmClearHistory() bool {
	return win(user, "MessageBoxW", settingsWindow, ptr(wtext("確定要清空所有已完成紀錄？")), ptr(wtext("清空完成紀錄")), 0x124) == 6
}

func scaled(n int) int { return n * currentScale / 100 }
func drawGrip(frame *image.RGBA) {
	for y := 282; y < 307; y++ {
		for x := 313; x < 338; x++ {
			frame.SetRGBA(x, y, color.RGBA{R: 62, G: 45, B: 42, A: 220})
			for i := 0; i < 3; i++ {
				if x >= 320 && y >= 288 && absInt(x+y-(627+i*5)) <= 1 {
					frame.SetRGBA(x, y, color.RGBA{R: 248, G: 230, B: 216, A: 255})
				}
			}
		}
	}
}
func absInt(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
func changeSpriteScale(scale int) {
	scale = clampSpriteScale(scale)
	if currentScale == scale {
		return
	}
	currentScale = scale
	var windowArea, work [4]int32
	win(user, "GetWindowRect", mascotWindow, ptr(&windowArea))
	monitor := win(user, "MonitorFromWindow", mascotWindow, 2)
	var monitorInfo struct {
		size          uint32
		monitor, work [4]int32
		flags         uint32
	}
	monitorInfo.size = uint32(unsafe.Sizeof(monitorInfo))
	if win(user, "GetMonitorInfoW", monitor, ptr(&monitorInfo)) != 0 {
		work = monitorInfo.work
		x := max(int(work[0]), min(int(windowArea[0]), int(work[2])-scaled(mascotWidth)))
		y := max(int(work[1]), min(int(windowArea[1]), int(work[3])-scaled(mascotHeight)))
		win(user, "SetWindowPos", mascotWindow, 0, uintptr(x), uintptr(y), 0, 0, 0x15)
	}
	drawMascot()
	for _, child := range []struct {
		window     uintptr
		x, y, w, h int
	}{
		{reminderTitle, 0, 2, 244, 32}, {reminderDetail, 0, 34, 244, 17}, {reminderConfirm, 76, 52, 92, 25}, {quoteBubbleText, 0, 5, 244, 70},
	} {
		win(user, "MoveWindow", child.window, uintptr(scaled(child.x)), uintptr(scaled(child.y)), uintptr(scaled(child.w)), uintptr(scaled(child.h)), 1)
	}
	positionReminder()
}
func reloadSprite() {
	spriteFrame = -1
	spriteEpoch = time.Now()
	paintMascot(bubblePick)
	setAppearanceLabel(spriteLabel())
}
func setAppearanceLabel(label string) {
	win(user, "SetWindowTextW", appearanceButton, ptr(wtext(label)))
}
func showAppearance() {
	popup := win(user, "CreatePopupMenu")
	for i, label := range []string{"蜜袋鼯（動畫）", "可愛狐狸（動畫）", "匯入 PNG 或 GIF…"} {
		win(user, "AppendMenuW", popup, 0, uintptr(i+1), ptr(wtext(label)))
	}
	var area [4]int32
	win(user, "GetWindowRect", appearanceButton, ptr(&area))
	choice := win(user, "TrackPopupMenu", popup, 0x102, uintptr(area[0]), uintptr(area[3]), 0, settingsWindow, 0)
	win(user, "DestroyMenu", popup)
	switch choice {
	case 1:
		chooseMascot("", "")
	case 2:
		chooseMascot("fox", "")
	case 3:
		if path := chooseImageFile(); path != "" {
			chooseMascot("", path)
		}
	}
}

type fileDialog struct {
	size                      uint32
	owner, instance           uintptr
	filter, customFilter      *uint16
	maxCustom, filterIndex    uint32
	file                      *uint16
	maxFile                   uint32
	fileTitle                 *uint16
	maxFileTitle              uint32
	initialDir, title         *uint16
	flags                     uint32
	fileOffset, fileExtension uint16
	defaultExt                *uint16
	customData, hook          uintptr
	templateName              *uint16
	reserved                  uintptr
	reservedFlags, flagsEx    uint32
}

func chooseImageFile() string {
	const dialogFlags = 0x1000 | 0x800 | 0x8 | 0x80000
	filter := append(syscall.StringToUTF16("角色圖示"), syscall.StringToUTF16("*.png;*.gif")...)
	filter = append(filter, 0)
	output := make([]uint16, 32768)
	dialog := fileDialog{owner: settingsWindow, filter: &filter[0], file: &output[0], maxFile: uint32(len(output)), title: wtext("選擇小精靈圖示"), flags: dialogFlags}
	dialog.size = uint32(unsafe.Sizeof(dialog))
	result := win(syscall.NewLazyDLL("comdlg32.dll"), "GetOpenFileNameW", ptr(&dialog))
	runtime.KeepAlive(&dialog)
	if result == 0 {
		return ""
	}
	return syscall.UTF16ToString(output)
}

var quoteWindow, quoteEdit, quoteBubbleText uintptr
var quoteOverlay bool

func showQuoteBubbleW() {
	quoteOverlay = true
	for _, control := range []uintptr{reminderTitle, reminderDetail, reminderConfirm} {
		win(user, "ShowWindow", control, 0)
	}
	win(user, "SetWindowTextW", quoteBubbleText, ptr(wtext(customSpeech(bubblePick))))
	win(user, "ShowWindow", quoteBubbleText, 5)
	positionReminder()
	win(user, "ShowWindow", reminderWindow, 4*boolW(spriteVisible))
}

func showQuoteEditor(text string) {
	if quoteWindow != 0 && win(user, "IsWindowVisible", quoteWindow) != 0 {
		win(user, "SetForegroundWindow", quoteWindow)
		return
	}
	if quoteWindow == 0 {
		var area [4]int32
		win(user, "GetWindowRect", settingsWindow, ptr(&area))
		quoteWindow = win(user, "CreateWindowExW", 0x80, ptr(wtext("ClockSpriteUnified")), ptr(wtext("幹話語錄")), 0x80CA0000, uintptr(area[0]), uintptr(area[1]), 625, 465, settingsWindow, 0, instance, 0)
		if quoteWindow == 0 {
			notice("無法開啟幹話語錄。")
			return
		}
		makeChild(quoteWindow, "STATIC", "每行一句。匯入會追加新語錄，按儲存後才會生效。", 0, 15, 14, 580, 23, 0)
		quoteEdit = makeChild(quoteWindow, "EDIT", "", 0xA11044, 15, 42, 575, 330, 0)
		win(user, "SendMessageW", quoteEdit, 0xC5, maxQuoteFile, 0)
		for i, button := range []struct {
			title string
			id    int
		}{
			{"匯入文字檔…", actionQuoteImport}, {"匯出文字檔…", actionQuoteExport}, {"儲存", actionQuoteSave},
		} {
			makeChild(quoteWindow, "BUTTON", button.title, 0x10000, uintptr(15+i*130), 386, 122, 29, uintptr(button.id))
		}
	}
	setQuoteEditorText(text)
	win(user, "ShowWindow", quoteWindow, 5)
	win(user, "SetForegroundWindow", quoteWindow)
	win(user, "SetFocus", quoteEdit)
}
func closeQuoteEditor() { win(user, "ShowWindow", quoteWindow, 0) }
func quoteEditorText() string {
	length := win(user, "GetWindowTextLengthW", quoteEdit)
	buffer := make([]uint16, length+1)
	win(user, "GetWindowTextW", quoteEdit, ptr(&buffer[0]), length+1)
	return syscall.UTF16ToString(buffer)
}
func setQuoteEditorText(text string) {
	win(user, "SetWindowTextW", quoteEdit, ptr(wtext(strings.ReplaceAll(text, "\n", "\r\n"))))
}
func quoteFile(save bool) string {
	filter := append(syscall.StringToUTF16("UTF-8 文字檔"), syscall.StringToUTF16("*.txt")...)
	filter = append(filter, 0)
	output := make([]uint16, 32768)
	method, title, flags := "GetOpenFileNameW", "匯入幹話語錄", uint32(0x81808)
	if save {
		method, title, flags = "GetSaveFileNameW", "匯出幹話語錄", 0x8080A
		copy(output, syscall.StringToUTF16("ClockSprite-幹話語錄.txt"))
	}
	dialog := fileDialog{owner: quoteWindow, filter: &filter[0], file: &output[0], maxFile: uint32(len(output)), title: wtext(title), flags: flags, defaultExt: wtext("txt")}
	dialog.size = uint32(unsafe.Sizeof(dialog))
	result := win(syscall.NewLazyDLL("comdlg32.dll"), method, ptr(&dialog))
	runtime.KeepAlive(&dialog)
	if result == 0 {
		return ""
	}
	return syscall.UTF16ToString(output)
}

var repeatButton uintptr

func repeatMode() bool { return win(user, "SendMessageW", repeatButton, 0xF0, 0, 0) == 1 }
func setRepeatMode(on bool) {
	win(user, "SendMessageW", repeatButton, 0xF1, boolW(on), 0)
	for _, field := range []int{fieldEndTime, fieldInterval} {
		win(user, "EnableWindow", fields[field], boolW(on))
	}
}
