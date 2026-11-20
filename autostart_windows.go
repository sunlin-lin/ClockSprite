package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runName = "ClockSprite"

var registry = syscall.NewLazyDLL("advapi32.dll")

func autostartChecked() bool { return win(user, "SendMessageW", startupButton, 0xF0, 0, 0) == 1 }
func refreshAutostart() {
	var key syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wtext(runKey), 0, syscall.KEY_QUERY_VALUE, &key) != nil {
		win(user, "SendMessageW", startupButton, 0xF1, 0, 0)
		return
	}
	defer syscall.RegCloseKey(key)
	var size uint32
	err := syscall.RegQueryValueEx(key, wtext(runName), nil, nil, nil, &size)
	win(user, "SendMessageW", startupButton, 0xF1, boolW(err == nil && size > 0), 0)
}
func setAutostart(enabled bool) error {
	var key syscall.Handle
	err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wtext(runKey), 0, syscall.KEY_SET_VALUE, &key)
	if !enabled && err == syscall.ERROR_FILE_NOT_FOUND {
		return nil
	}
	if err != nil {
		return fmt.Errorf("無法開啟登入啟動設定：%w", err)
	}
	defer syscall.RegCloseKey(key)
	name := wtext(runName)
	if !enabled {
		code := win(registry, "RegDeleteValueW", uintptr(key), ptr(name))
		if code != 0 && code != uintptr(syscall.ERROR_FILE_NOT_FOUND) {
			return fmt.Errorf("無法停用登入啟動：%w", syscall.Errno(code))
		}
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	value := syscall.StringToUTF16("\"" + executable + "\"")
	code := win(registry, "RegSetValueExW", uintptr(key), ptr(name), 0, syscall.REG_SZ, uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)*2))
	if code != 0 {
		return fmt.Errorf("無法啟用登入啟動：%w", syscall.Errno(code))
	}
	return nil
}
