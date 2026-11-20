package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const startupAgent = "com.clocksprite.desktop.login"

func agentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", startupAgent+".plist"), nil
}
func autostartChecked() bool { return send(startupButton, "state") == 1 }
func refreshAutostart() {
	path, err := agentPath()
	checked := int64(0)
	if err == nil {
		if _, err = os.Stat(path); err == nil {
			checked = 1
		}
	}
	send(startupButton, "setState:", checked)
}
func setAutostart(enabled bool) error {
	path, err := agentPath()
	if err != nil {
		return err
	}
	if !enabled {
		err = os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if !strings.HasSuffix(bundle, ".app") {
		return fmt.Errorf("請從 ClockSprite.app 啟動後再設定登入啟動")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	var escaped bytes.Buffer
	if err = xml.EscapeText(&escaped, []byte(bundle)); err != nil {
		return err
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>/usr/bin/open</string><string>-a</string><string>%s</string></array>
<key>RunAtLoad</key><true/>
</dict></plist>
`, startupAgent, escaped.String())
	if err = os.WriteFile(path+".tmp", []byte(plist), 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}
