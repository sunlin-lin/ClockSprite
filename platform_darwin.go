package main

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type size struct{ W, H float64 }
type position struct{ X, Y float64 }
type rect struct {
	Origin position
	Size   size
}

func frame(x, y, w, h float64) rect                         { return rect{position{x, y}, size{w, h}} }
func sel(name string) objc.SEL                              { return objc.RegisterName(name) }
func send(object objc.ID, name string, args ...any) objc.ID { return object.Send(sel(name), args...) }
func cls(name string) objc.ID                               { return objc.ID(objc.GetClass(name)) }
func str(value string) objc.ID                              { return send(cls("NSString"), "stringWithUTF8String:", value+"\x00") }

var application, controller, settingsWindow, mascotWindow, bubbleView, status, mode, table, menu objc.ID
var mascotView, spriteTimer, gripClass, appearanceButton objc.ID
var resizeAnchor position
var initialScale int
var currentScale = 100
var spriteImages []objc.ID
var spriteEpoch time.Time
var spriteVisible bool
var spriteFrame = -1
var pendingTab, historyTab, completeButton, editButton, deleteButton, clearHistoryButton, forceClockButton objc.ID
var reminderTitle, reminderDetail, reminderButton, reminderButtonClass objc.ID
var dateButton, startupButton, calendarWindow, calendarTitle objc.ID
var calendarCells [42]objc.ID
var fields = map[int]objc.ID{}
var dayButtons [7]objc.ID
var tableRows []string
var imageClass objc.ID

func legacyClockPath(root string) string { return filepath.Join(root, "ClockSprite", "state.json") }

var errRunning = errors.New("小精靈已在執行")

func acquireLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errRunning
		}
		return nil, err
	}
	return file, nil
}
func notice(message string) {
	alert := send(cls("NSAlert"), "new")
	send(alert, "setMessageText:", str("社畜小精靈"))
	send(alert, "setInformativeText:", str(message))
	send(alert, "runModal")
	send(alert, "release")
}
func loadArtImage(path string) objc.ID {
	raw, err := resources.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return 0
	}
	return imageData(raw)
}
func imageData(raw []byte) objc.ID {
	data := send(cls("NSData"), "dataWithBytes:length:", unsafe.Pointer(&raw[0]), uintptr(len(raw)))
	runtime.KeepAlive(raw)
	return send(send(cls("NSImage"), "alloc"), "initWithData:", data)
}
func window(name string, dims rect, style uint64) objc.ID {
	result := send(send(cls("NSWindow"), "alloc"), "initWithContentRect:styleMask:backing:defer:", dims, style, uint64(2), false)
	send(result, "setReleasedWhenClosed:", false)
	send(result, "setTitle:", str(name))
	return result
}
func control(parent, kind objc.ID, dims rect) objc.ID {
	view := send(send(kind, "alloc"), "initWithFrame:", dims)
	send(parent, "addSubview:", view)
	send(view, "release")
	return view
}
func label(parent objc.ID, value string, dims rect) objc.ID {
	field := control(parent, cls("NSTextField"), dims)
	send(field, "setEditable:", false)
	send(field, "setSelectable:", false)
	send(field, "setBezeled:", false)
	send(field, "setDrawsBackground:", false)
	send(field, "setStringValue:", str(value))
	return field
}

// The desktop window is transparent and normally inactive. Give its controls
// their own colors so AppKit's inactive appearance cannot hide the text.
func spriteColor(red, green, blue float64) objc.ID {
	return send(cls("NSColor"), "colorWithSRGBRed:green:blue:alpha:", red, green, blue, 1.0)
}

func styleSpriteButton(button objc.ID, title string, fontSize float64) {
	send(button, "setBordered:", false)
	send(button, "setWantsLayer:", true)
	layer := send(button, "layer")
	send(layer, "setBackgroundColor:", send(spriteColor(0.19, 0.25, 0.34), "CGColor"))
	send(layer, "setBorderColor:", send(spriteColor(0.90, 0.92, 0.95), "CGColor"))
	send(layer, "setBorderWidth:", 1.0)
	send(layer, "setCornerRadius:", 6.0)

	attributes := send(cls("NSMutableDictionary"), "dictionary")
	send(attributes, "setObject:forKey:", send(cls("NSColor"), "whiteColor"), str("NSColor"))
	send(attributes, "setObject:forKey:", send(cls("NSFont"), "boldSystemFontOfSize:", fontSize), str("NSFont"))
	styled := send(send(cls("NSAttributedString"), "alloc"), "initWithString:attributes:", str(title), attributes)
	send(button, "setTitle:", str(title))
	send(button, "setAttributedTitle:", styled)
	send(styled, "release")
}

func edit(parent objc.ID, id int, dims rect) {
	fields[id] = control(parent, cls("NSTextField"), dims)
}
func button(parent objc.ID, title string, id int, dims rect, check bool) objc.ID {
	object := control(parent, cls("NSButton"), dims)
	if check {
		send(object, "setButtonType:", int64(3))
	} else {
		send(object, "setBezelStyle:", int64(1))
	}
	send(object, "setTitle:", str(title))
	send(object, "setTag:", int64(id))
	send(object, "setTarget:", controller)
	send(object, "setAction:", sel("performChecklistAction:"))
	return object
}
func setInput(id int, value string) { send(fields[id], "setStringValue:", str(value)) }
func input(id int) string {
	return objc.Send[string](send(fields[id], "stringValue"), sel("UTF8String"))
}
func setChecked(day int, on bool) {
	state := int64(0)
	if on {
		state = 1
	}
	send(dayButtons[day], "setState:", state)
}
func checked(day int) bool { return send(dayButtons[day], "state") == 1 }
func setMode(on bool) {
	state := int64(0)
	if on {
		state = 1
	}
	send(mode, "setState:", state)
	modeChanged()
}
func weeklyMode() bool { return send(mode, "state") == 1 }
func modeChanged() {
	on := weeklyMode()
	send(fields[fieldDate], "setEnabled:", !on)
	send(dateButton, "setEnabled:", !on)
	if on {
		showCalendar(false)
	}
	for _, day := range dayButtons {
		send(day, "setEnabled:", on)
	}
}
func setStatus(value string) { send(status, "setStringValue:", str(value)) }
func setRows(values []string, previous int) {
	tableRows = values
	send(table, "reloadData")
	if len(values) > 0 {
		index := uint64(max(0, min(previous, len(values)-1)))
		set := send(cls("NSIndexSet"), "indexSetWithIndex:", index)
		send(table, "selectRowIndexes:byExtendingSelection:", set, false)
	}
}
func selected() int { return int(send(table, "selectedRow")) }
func showSettings() {
	send(application, "activateIgnoringOtherApps:", true)
	send(settingsWindow, "makeKeyAndOrderFront:", objc.ID(0))
}
func showMascot(on bool) {
	spriteVisible = on
	updateSpriteTimer()
	if on {
		send(mascotWindow, "orderFrontRegardless")
	} else {
		send(mascotWindow, "orderOut:", objc.ID(0))
	}
}
func paintMascot(pick int) {
	if activeReminder.action != "" {
		setSpriteFrame(0)
	}
	updateSpriteTimer()
	quote := customSpeech(pick)
	send(quoteBubbleText, "setStringValue:", str(quote))
	send(quoteBubbleText, "setHidden:", quote == "")
	path := ""
	switch {
	case pick == -2 || quote != "":
		path = "art/reminder_cloud.png"
	case pick >= 0:
		path = fmt.Sprintf("art/bubbles/%03d.png", pick)
	}
	if path == "" {
		send(bubbleView, "setImage:", objc.ID(0))
		return
	}
	picture := loadArtImage(path)
	if picture != 0 {
		send(bubbleView, "setImage:", picture)
		send(picture, "release")
	}
}
func register() error {
	methods := []objc.MethodDef{
		{Cmd: sel("performChecklistAction:"), Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) { onAction(int(send(sender, "tag"))) }},
		{Cmd: sel("spriteAnimate:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { animateSprite() }},
		{Cmd: sel("spriteTick:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { tick() }},
		{Cmd: sel("spriteSettings:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { openSettings() }},
		{Cmd: sel("spriteShow:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { hideMascot(false) }},
		{Cmd: sel("spriteHide:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { hideMascot(true) }},
		{Cmd: sel("spriteQuit:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { send(application, "terminate:", objc.ID(0)) }},
		{Cmd: sel("applicationShouldHandleReopen:hasVisibleWindows:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, _ bool) bool { openSettings(); return false }},
		{Cmd: sel("windowShouldClose:"), Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) bool {
			if sender == calendarWindow {
				showCalendar(false)
				return false
			}
			showCalendar(false)
			return true
		}},
		{Cmd: sel("numberOfRowsInTableView:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) int64 { return int64(len(tableRows)) }},
		{Cmd: sel("tableView:objectValueForTableColumn:row:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID, _ objc.ID, index int64) objc.ID {
			if index < 0 || index >= int64(len(tableRows)) {
				return 0
			}
			return str(tableRows[index])
		}},
	}
	obj, err := objc.RegisterClass("ClockSpriteUnifiedController", objc.GetClass("NSObject"), nil, nil, methods)
	if err != nil {
		return err
	}
	controller = send(objc.ID(obj), "new")
	send(application, "setDelegate:", controller)
	view, err := objc.RegisterClass("ClockSpriteUnifiedImage", objc.GetClass("NSImageView"), nil, nil, []objc.MethodDef{
		{Cmd: sel("acceptsFirstMouse:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) bool { return true }},
		{Cmd: sel("mouseDown:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
			send(send(self, "window"), "performWindowDragWithEvent:", event)
		}},
		{Cmd: sel("rightMouseDown:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
			send(cls("NSMenu"), "popUpContextMenu:withEvent:forView:", menu, event, self)
		}},
	})
	imageClass = objc.ID(view)
	if err != nil {
		return err
	}
	buttonClass, err := objc.RegisterClass("ClockSpriteReminderButton", objc.GetClass("NSButton"), nil, nil, []objc.MethodDef{
		{Cmd: sel("acceptsFirstMouse:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) bool { return true }},
	})
	reminderButtonClass = objc.ID(buttonClass)
	if err != nil {
		return err
	}
	grip, err := objc.RegisterClass("ClockSpriteResizeGrip", objc.GetClass("NSButton"), nil, nil, []objc.MethodDef{
		{Cmd: sel("acceptsFirstMouse:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) bool { return true }},
		{Cmd: sel("mouseDown:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
			resizeAnchor = objc.Send[position](cls("NSEvent"), sel("mouseLocation"))
			initialScale = currentScale
		}},
		{Cmd: sel("mouseDragged:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) {
			now := objc.Send[position](cls("NSEvent"), sel("mouseLocation"))
			changeSpriteScale(dragScale(initialScale, int(now.X-resizeAnchor.X), int(resizeAnchor.Y-now.Y)))
		}},
		{Cmd: sel("mouseUp:"), Fn: func(_ objc.ID, _ objc.SEL, _ objc.ID) { savedSpriteScale(currentScale) }},
	})
	gripClass = objc.ID(grip)
	return err
}
func build() {
	screen := send(cls("NSScreen"), "mainScreen")
	area := objc.Send[rect](screen, sel("visibleFrame"))
	x := area.Origin.X + area.Size.W
	y := area.Origin.Y
	mascotWindow = window("社畜小精靈", frame(x-364, y+24, 340, 310), 0)
	send(mascotWindow, "setOpaque:", false)
	send(mascotWindow, "setHasShadow:", false)
	send(mascotWindow, "setBackgroundColor:", send(cls("NSColor"), "clearColor"))
	send(mascotWindow, "setLevel:", int64(3))
	send(mascotWindow, "setCollectionBehavior:", uint64(1|16|256))
	parent := send(mascotWindow, "contentView")
	send(parent, "setWantsLayer:", true)
	mascotView = control(parent, imageClass, frame(spriteX, 310-spriteY-spriteH, spriteW, spriteH))
	spriteImages = make([]objc.ID, len(activeSprite().frames))
	if len(spriteImages) > 0 {
		setSpriteFrame(0)
	} else {
		portrait := loadArtImage("art/floating_mascot.png")
		send(mascotView, "setImage:", portrait)
		send(portrait, "release")
	}
	bubbleView = control(parent, imageClass, frame(0, 176, 340, 134))
	quoteBubbleText = label(parent, "", frame(48, 207, 244, 76))
	send(quoteBubbleText, "setAlignment:", int64(2))
	send(quoteBubbleText, "setTextColor:", spriteColor(0.16, 0.12, 0.11))
	send(quoteBubbleText, "setLineBreakMode:", int64(1))
	buildReminder(parent)
	grip := control(parent, gripClass, frame(307, 3, 30, 30))
	styleSpriteButton(grip, "↘", 19)
	send(grip, "setToolTip:", str("拖曳調整大小"))
	changeSpriteScale(storedSpriteScale())
	settingsWindow = window("社畜小精靈設定", frame(x-665, y+60, 625, 555), 3)
	send(settingsWindow, "setDelegate:", controller)
	parent = send(settingsWindow, "contentView")
	button(parent, "幹話語錄", actionQuoteSettings, frame(15, 512, 143, 32), false)
	forceClockButton = button(parent, "強制提醒上下班打卡", 0, frame(190, 517, 236, 25), true)
	appearanceButton = button(parent, "角色圖示", actionAppearance, frame(437, 512, 172, 32), false)
	label(parent, "上班", frame(18, 480, 40, 22))
	edit(parent, fieldIn, frame(62, 481, 70, 23))
	label(parent, "下班", frame(145, 480, 40, 22))
	edit(parent, fieldOut, frame(190, 481, 70, 23))
	label(parent, "語錄間隔（分鐘）", frame(272, 480, 130, 22))
	edit(parent, fieldSpeech, frame(403, 481, 58, 23))
	button(parent, "儲存打卡設定", actionClock, frame(470, 476, 139, 30), false)
	status = label(parent, "", frame(18, 451, 345, 22))
	startupButton = button(parent, "登入電腦時自動啟動", actionAutostart, frame(375, 449, 233, 25), true)
	label(parent, "自訂提醒", frame(18, 416, 120, 22))
	label(parent, "提醒名稱", frame(18, 385, 70, 22))
	edit(parent, fieldTitle, frame(89, 385, 520, 23))
	mode = button(parent, "每週固定", actionMode, frame(18, 352, 94, 24), true)
	label(parent, "日期", frame(118, 352, 40, 22))
	edit(parent, fieldDate, frame(158, 352, 118, 23))
	send(fields[fieldDate], "setEditable:", false)
	dateButton = button(parent, "選日期", actionPickDate, frame(280, 348, 96, 30), false)
	label(parent, "時間", frame(397, 352, 42, 22))
	edit(parent, fieldTime, frame(444, 352, 80, 23))
	for i, day := range weekdayOrder {
		dayButtons[day] = button(parent, "週"+weekdays[day], 0, frame(18+float64(i)*68, 319, 66, 24), true)
	}
	button(parent, "清空", actionClear, frame(15, 279, 110, 30), false)
	button(parent, "儲存提醒", actionSave, frame(127, 279, 110, 30), false)
	repeatButton = button(parent, "時段內重複", actionRepeatMode, frame(247, 281, 109, 25), true)
	label(parent, "到", frame(361, 282, 20, 23))
	edit(parent, fieldEndTime, frame(383, 282, 68, 23))
	label(parent, "每隔", frame(461, 282, 36, 23))
	edit(parent, fieldInterval, frame(502, 282, 48, 23))
	label(parent, "分鐘", frame(557, 282, 48, 23))
	pendingTab = button(parent, "待辦與排程", actionShowPending, frame(18, 247, 113, 27), true)
	historyTab = button(parent, "完成紀錄", actionShowHistory, frame(140, 247, 108, 27), true)
	send(pendingTab, "setButtonType:", int64(4))
	send(historyTab, "setButtonType:", int64(4))
	scroll := control(parent, cls("NSScrollView"), frame(18, 66, 590, 181))
	send(scroll, "setHasVerticalScroller:", true)
	send(scroll, "setBorderType:", uint64(1))
	table = send(send(cls("NSTableView"), "alloc"), "initWithFrame:", frame(0, 0, 582, 175))
	column := send(send(cls("NSTableColumn"), "alloc"), "initWithIdentifier:", str("reminder"))
	send(column, "setWidth:", float64(565))
	send(table, "addTableColumn:", column)
	send(column, "release")
	send(table, "setHeaderView:", objc.ID(0))
	send(table, "setUsesAlternatingRowBackgroundColors:", true)
	send(table, "setDataSource:", controller)
	send(scroll, "setDocumentView:", table)
	send(table, "release")
	completeButton = button(parent, "勾選完成", actionDone, frame(15, 25, 145, 30), false)
	editButton = button(parent, "編輯排程", actionEdit, frame(164, 25, 107, 30), false)
	deleteButton = button(parent, "刪除排程", actionDelete, frame(274, 25, 107, 30), false)
	clearHistoryButton = button(parent, "清空紀錄", actionClearHistory, frame(164, 25, 107, 30), false)
	buildCalendar()
	bar := send(cls("NSStatusBar"), "systemStatusBar")
	statusItem := send(send(bar, "statusItemWithLength:", float64(-1)), "retain")
	trayButton := send(statusItem, "button")
	icon := loadArtImage("art/tray_mascot.png")
	if icon != 0 {
		send(icon, "setSize:", size{18, 18})
		send(trayButton, "setImage:", icon)
		send(icon, "release")
	} else {
		send(trayButton, "setTitle:", str("社"))
	}
	send(trayButton, "setToolTip:", str("社畜小精靈"))
	menu = send(cls("NSMenu"), "new")
	for _, item := range []struct{ name, action string }{{"設定", "spriteSettings:"}, {"顯示小精靈", "spriteShow:"}, {"隱藏小精靈", "spriteHide:"}, {"結束社畜小精靈", "spriteQuit:"}} {
		entry := send(menu, "addItemWithTitle:action:keyEquivalent:", str(item.name), sel(item.action), str(""))
		send(entry, "setTarget:", controller)
	}
	send(statusItem, "setMenu:", menu)
}
func main() {
	runtime.LockOSThread()
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	pool := send(cls("NSAutoreleasePool"), "new")
	application = send(cls("NSApplication"), "sharedApplication")
	send(application, "setActivationPolicy:", int64(1))
	lock, err := load()
	if err != nil {
		if errors.Is(err, errRunning) {
			notice("請先結束舊版提醒清單，再開啟新版小精靈。")
		} else {
			notice(err.Error())
		}
		return
	}
	defer lock.Close()
	if err = register(); err != nil {
		notice(err.Error())
		return
	}
	build()
	newRule()
	openFields()
	send(application, "finishLaunching")
	if !clock.Hidden && activeReminder.action == "" {
		speakNow()
		showMascot(true)
	}
	tick()
	send(cls("NSTimer"), "scheduledTimerWithTimeInterval:target:selector:userInfo:repeats:", float64(2), controller, sel("spriteTick:"), objc.ID(0), true)
	send(pool, "drain")
	send(application, "run")
}

func buildCalendar() {
	calendarWindow = window("選擇日期", frame(100, 100, 320, 316), 3)
	send(calendarWindow, "setDelegate:", controller)
	parent := send(calendarWindow, "contentView")
	button(parent, "上個月", actionMonthBack, frame(13, 276, 76, 27), false)
	calendarTitle = label(parent, "", frame(91, 275, 138, 24))
	send(calendarTitle, "setAlignment:", int64(2))
	button(parent, "下個月", actionMonthNext, frame(229, 276, 76, 27), false)
	for i, name := range weekdays {
		caption := label(parent, name, frame(18+41*float64(i), 246, 38, 21))
		send(caption, "setAlignment:", int64(2))
	}
	for i := range calendarCells {
		calendarCells[i] = button(parent, "", actionCalendarDay+i, frame(18+41*float64(i%7), 212-30*float64(i/7), 38, 27), false)
	}
	button(parent, "取消", actionCalendarCancel, frame(228, 15, 78, 27), false)
}
func renderCalendar(title string, labels [42]string, enabled [42]bool) {
	send(calendarTitle, "setStringValue:", str(title))
	for i, cell := range calendarCells {
		send(cell, "setTitle:", str(labels[i]))
		send(cell, "setEnabled:", enabled[i])
		send(cell, "setHidden:", labels[i] == "")
	}
}
func showCalendar(open bool) {
	if calendarWindow == 0 {
		return
	}
	if !open {
		if send(calendarWindow, "parentWindow") == settingsWindow {
			send(settingsWindow, "removeChildWindow:", calendarWindow)
			send(calendarWindow, "orderOut:", objc.ID(0))
			send(settingsWindow, "makeKeyWindow")
		}
		return
	}
	area := objc.Send[rect](settingsWindow, sel("frame"))
	send(calendarWindow, "setFrameOrigin:", position{area.Origin.X + (area.Size.W-320)/2, area.Origin.Y + (area.Size.H-316)/2})
	if send(calendarWindow, "parentWindow") == 0 {
		send(settingsWindow, "addChildWindow:ordered:", calendarWindow, int64(1))
	}
	send(calendarWindow, "makeKeyAndOrderFront:", objc.ID(0))
}

func buildReminder(parent objc.ID) {
	reminderTitle = label(parent, "", frame(48, 236, 244, 41))
	send(reminderTitle, "setAlignment:", int64(2))
	send(reminderTitle, "setLineBreakMode:", int64(4))
	reminderDetail = label(parent, "", frame(48, 215, 244, 19))
	send(reminderDetail, "setAlignment:", int64(2))
	for _, field := range []objc.ID{reminderTitle, reminderDetail} {
		send(field, "setTextColor:", spriteColor(0.16, 0.12, 0.11))
	}
	reminderButton = control(parent, reminderButtonClass, frame(120, 188, 100, 29))
	styleSpriteButton(reminderButton, "確認", 14)
	send(reminderButton, "setTag:", int64(actionConfirmReminder))
	send(reminderButton, "setTarget:", controller)
	send(reminderButton, "setAction:", sel("performChecklistAction:"))
	hideReminderBubble()
}
func showReminderBubble(title, detail string) {
	paintMascot(-2)
	send(reminderTitle, "setStringValue:", str(title))
	send(reminderDetail, "setStringValue:", str(detail))
	for _, view := range []objc.ID{reminderTitle, reminderDetail, reminderButton} {
		send(view, "setHidden:", false)
	}
}
func hideReminderBubble() {
	for _, view := range []objc.ID{reminderTitle, reminderDetail, reminderButton} {
		send(view, "setHidden:", true)
	}
}

func setHistoryView(history bool) {
	pendingState, historyState, title := int64(1), int64(0), "勾選完成"
	if history {
		pendingState, historyState, title = 0, 1, "取消完成"
	}
	send(pendingTab, "setState:", pendingState)
	send(historyTab, "setState:", historyState)
	send(completeButton, "setTitle:", str(title))
	send(editButton, "setHidden:", history)
	send(deleteButton, "setHidden:", history)
	send(clearHistoryButton, "setHidden:", !history)
}

func forceClockChecked() bool { return send(forceClockButton, "state") == 1 }
func setForceClockChecked(on bool) {
	state := int64(0)
	if on {
		state = 1
	}
	send(forceClockButton, "setState:", state)
}
func confirmClearHistory() bool {
	alert := send(cls("NSAlert"), "new")
	defer send(alert, "release")
	send(alert, "setMessageText:", str("確定要清空所有已完成紀錄？"))
	send(alert, "setAlertStyle:", int64(0))
	send(alert, "addButtonWithTitle:", str("取消"))
	send(alert, "addButtonWithTitle:", str("清空紀錄"))
	return send(alert, "runModal") == 1001
}

func updateSpriteTimer() {
	play := spriteVisible && activeReminder.action == "" && len(spriteImages) > 1
	if play && spriteTimer == 0 {
		spriteEpoch = time.Now()
		spriteTimer = send(cls("NSTimer"), "scheduledTimerWithTimeInterval:target:selector:userInfo:repeats:", float64(.025), controller, sel("spriteAnimate:"), objc.ID(0), true)
	} else if !play && spriteTimer != 0 {
		send(spriteTimer, "invalidate")
		spriteTimer = 0
	}
}
func animateSprite() {
	if spriteTimer != 0 {
		setSpriteFrame(activeSprite().index(time.Since(spriteEpoch)))
	}
}
func setSpriteFrame(index int) {
	if index < 0 || index >= len(spriteImages) || index == spriteFrame {
		return
	}
	picture := spriteImages[index]
	if picture == 0 {
		var data bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		if encoder.Encode(&data, activeSprite().frames[index]) != nil {
			return
		}
		pool := send(cls("NSAutoreleasePool"), "new")
		picture = imageData(data.Bytes())
		send(pool, "drain")
		if picture == 0 {
			return
		}
		spriteImages[index] = picture
	}
	send(mascotView, "setImage:", picture)
	spriteFrame = index
}

func changeSpriteScale(scale int) {
	scale = clampSpriteScale(scale)
	if currentScale == scale {
		return
	}
	currentScale = scale
	old := objc.Send[rect](mascotWindow, sel("frame"))
	width := float64(mascotWidth) * float64(scale) / 100
	height := float64(mascotHeight) * float64(scale) / 100
	x, y := old.Origin.X, old.Origin.Y+old.Size.H-height
	screen := send(mascotWindow, "screen")
	if screen != 0 {
		area := objc.Send[rect](screen, sel("visibleFrame"))
		x = max(area.Origin.X, min(x, area.Origin.X+area.Size.W-width))
		y = max(area.Origin.Y, min(y, area.Origin.Y+area.Size.H-height))
	}
	send(mascotWindow, "setFrame:display:", frame(x, y, width, height), true)
	send(send(mascotWindow, "contentView"), "setBoundsSize:", size{mascotWidth, mascotHeight})
}
func reloadSprite() {
	for _, picture := range spriteImages {
		if picture != 0 {
			send(picture, "release")
		}
	}
	spriteImages = make([]objc.ID, len(activeSprite().frames))
	spriteFrame = -1
	spriteEpoch = time.Now()
	if len(spriteImages) > 0 {
		setSpriteFrame(0)
	}
	updateSpriteTimer()
	setAppearanceLabel(spriteLabel())
}
func setAppearanceLabel(label string) { send(appearanceButton, "setTitle:", str(label)) }
func showAppearance() {
	alert := send(cls("NSAlert"), "new")
	defer send(alert, "release")
	send(alert, "setMessageText:", str("選擇小精靈圖示"))
	for _, title := range []string{"蜜袋鼯（動畫）", "可愛狐狸（動畫）", "匯入 PNG 或 GIF…", "取消"} {
		send(alert, "addButtonWithTitle:", str(title))
	}
	switch send(alert, "runModal") {
	case 1000:
		chooseMascot("", "")
	case 1001:
		chooseMascot("fox", "")
	case 1002:
		panel := send(cls("NSOpenPanel"), "openPanel")
		send(panel, "setAllowsMultipleSelection:", false)
		send(panel, "setCanChooseDirectories:", false)
		types := send(cls("NSMutableArray"), "array")
		send(types, "addObject:", str("png"))
		send(types, "addObject:", str("gif"))
		send(panel, "setAllowedFileTypes:", types)
		if send(panel, "runModal") == 1 {
			url := send(panel, "URL")
			path := objc.Send[string](send(url, "path"), sel("UTF8String"))
			if path != "" {
				chooseMascot("", path)
			}
		}
	}
}

var quoteWindow, quoteEdit, quoteBubbleText objc.ID

func showQuoteEditor(text string) {
	if quoteWindow != 0 && objc.Send[bool](quoteWindow, sel("isVisible")) {
		send(quoteWindow, "makeKeyAndOrderFront:", objc.ID(0))
		return
	}
	if quoteWindow == 0 {
		area := objc.Send[rect](settingsWindow, sel("frame"))
		quoteWindow = window("幹話語錄", frame(area.Origin.X, area.Origin.Y, 610, 443), 3)
		parent := send(quoteWindow, "contentView")
		label(parent, "每行一句。匯入會追加新語錄，按儲存後才會生效。", frame(15, 408, 580, 22))
		scroll := control(parent, cls("NSScrollView"), frame(15, 52, 580, 347))
		send(scroll, "setHasVerticalScroller:", true)
		send(scroll, "setBorderType:", int64(2))
		quoteEdit = send(send(cls("NSTextView"), "alloc"), "initWithFrame:", frame(0, 0, 565, 345))
		send(quoteEdit, "setRichText:", false)
		send(quoteEdit, "setVerticallyResizable:", true)
		send(quoteEdit, "setAutoresizingMask:", uint64(2))
		send(send(quoteEdit, "textContainer"), "setWidthTracksTextView:", true)
		send(scroll, "setDocumentView:", quoteEdit)
		send(quoteEdit, "release")
		button(parent, "匯入文字檔…", actionQuoteImport, frame(12, 12, 128, 31), false)
		button(parent, "匯出文字檔…", actionQuoteExport, frame(145, 12, 128, 31), false)
		button(parent, "儲存", actionQuoteSave, frame(490, 12, 108, 31), false)
	}
	setQuoteEditorText(text)
	send(application, "activateIgnoringOtherApps:", true)
	send(quoteWindow, "makeKeyAndOrderFront:", objc.ID(0))
	send(quoteWindow, "makeFirstResponder:", quoteEdit)
}
func closeQuoteEditor() { send(quoteWindow, "orderOut:", objc.ID(0)) }
func quoteEditorText() string {
	return objc.Send[string](send(quoteEdit, "string"), sel("UTF8String"))
}
func setQuoteEditorText(text string) { send(quoteEdit, "setString:", str(text)) }
func quoteFile(save bool) string {
	panel := send(cls("NSOpenPanel"), "openPanel")
	if save {
		panel = send(cls("NSSavePanel"), "savePanel")
		send(panel, "setNameFieldStringValue:", str("ClockSprite-幹話語錄.txt"))
	} else {
		send(panel, "setAllowsMultipleSelection:", false)
		send(panel, "setCanChooseDirectories:", false)
	}
	types := send(cls("NSMutableArray"), "array")
	send(types, "addObject:", str("txt"))
	send(panel, "setAllowedFileTypes:", types)
	if send(panel, "runModal") != 1 {
		return ""
	}
	return objc.Send[string](send(send(panel, "URL"), "path"), sel("UTF8String"))
}

var repeatButton objc.ID

func repeatMode() bool { return send(repeatButton, "state") == 1 }
func setRepeatMode(on bool) {
	state := int64(0)
	if on {
		state = 1
	}
	send(repeatButton, "setState:", state)
	for _, field := range []int{fieldEndTime, fieldInterval} {
		send(fields[field], "setEnabled:", on)
	}
}
