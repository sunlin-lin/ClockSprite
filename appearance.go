package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const mascotWidth, mascotHeight = 340, 310

var cachedAppearance string
var cachedSprite mascotAnimation

func clampSpriteScale(scale int) int { return max(60, min(175, scale)) }
func storedSpriteScale() int {
	if clock.SpriteScale == 0 {
		return 100
	}
	return clampSpriteScale(clock.SpriteScale)
}
func savedSpriteScale(scale int) {
	next := clock
	next.SpriteScale = clampSpriteScale(scale)
	if err := saveClockSettings(next); err != nil {
		notice(err.Error())
	}
}
func activeSprite() mascotAnimation {
	if clock.Mascot == "" {
		return mascotFrames()
	}
	if cachedAppearance == clock.Mascot && len(cachedSprite.frames) > 0 {
		return cachedSprite
	}
	var data []byte
	var err error
	if clock.Mascot == "fox" {
		data, err = resources.ReadFile("art/fox_mascot.gif")
	} else {
		if customIconPath(clock.Mascot) == "" {
			return mascotFrames()
		}
		data, err = os.ReadFile(customIconPath(clock.Mascot))
	}
	if err != nil {
		return mascotFrames()
	}
	result, err := decodeMascot(data)
	if err != nil {
		return mascotFrames()
	}
	cachedAppearance, cachedSprite = clock.Mascot, result
	return result
}
func customIconPath(style string) string {
	value, ok := strings.CutPrefix(style, "custom:")
	if !ok || len(value) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "Mascots", value)
}

func decodeMascot(raw []byte) (mascotAnimation, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 1024 || config.Height > 1024 {
		return mascotAnimation{}, errors.New("請選擇有效的 PNG 或 GIF，寬高最多 1024 像素。")
	}
	switch format {
	case "png":
		still, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			return mascotAnimation{}, err
		}
		return mascotAnimation{frames: []*image.RGBA{placeMascot(still)}, ends: []time.Duration{time.Second}}, nil
	case "gif":
		if config.Width*config.Height > 512*512 {
			return mascotAnimation{}, errors.New("GIF 的畫面面積最多為 512 × 512 像素。")
		}
		moving, err := gif.DecodeAll(bytes.NewReader(raw))
		if err != nil {
			return mascotAnimation{}, err
		}
		if len(moving.Image) > 240 {
			return mascotAnimation{}, errors.New("GIF 最多支援 240 幀。")
		}
		return prepareMascot(moving), nil
	}
	return mascotAnimation{}, errors.New("只能使用 PNG 或 GIF。")
}
func chooseMascot(style string, path string) {
	if path != "" {
		const maxBytes = 8 << 20
		f, err := os.Open(path)
		if err != nil {
			notice(err.Error())
			return
		}
		raw, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
		f.Close()
		if err != nil {
			notice(err.Error())
			return
		}
		if len(raw) > maxBytes {
			notice("角色圖示檔案最多 8 MB。")
			return
		}
		next, err := decodeMascot(raw)
		if err != nil {
			notice(err.Error())
			return
		}
		style = fmt.Sprintf("custom:%x", sha256.Sum256(raw))
		target := customIconPath(style)
		if err = os.MkdirAll(filepath.Dir(target), 0700); err == nil {
			if _, infoErr := os.Stat(target); os.IsNotExist(infoErr) {
				if err = os.WriteFile(target+".tmp", raw, 0600); err == nil {
					err = os.Rename(target+".tmp", target)
				}
			}
		}
		if err != nil {
			notice(err.Error())
			return
		}
		cachedAppearance, cachedSprite = style, next
	}
	next := clock
	next.Mascot = style
	if err := saveClockSettings(next); err != nil {
		notice(err.Error())
		return
	}
	reloadSprite()
}

func spriteLabel() string {
	if clock.Mascot == "fox" {
		return "角色圖示：可愛狐狸"
	}
	if strings.HasPrefix(clock.Mascot, "custom:") {
		return "角色圖示：自訂"
	}
	return "角色圖示：蜜袋鼯"
}

func dragScale(initial, horizontal, vertical int) int {
	x, y := horizontal*100/mascotWidth, vertical*100/mascotHeight
	if y*y > x*x {
		x = y
	}
	return clampSpriteScale(initial + x)
}
