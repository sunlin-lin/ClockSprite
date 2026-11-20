package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"
)

func TestEmbeddedMascotLoop(t *testing.T) {
	a := mascotFrames()
	if len(a.frames) != 156 {
		t.Fatalf("want 156 frames, got %d", len(a.frames))
	}
	duration := a.ends[len(a.ends)-1]
	if duration != 6760*time.Millisecond {
		t.Fatalf("wrong GIF duration: %v", duration)
	}
	if a.index(0) != 0 || a.index(a.ends[0]) != 1 || a.index(duration) != 0 {
		t.Fatal("invalid frame selection at boundaries")
	}
	if !bytes.Equal(a.frames[0].Pix, a.frames[len(a.frames)-1].Pix) {
		t.Fatal("loop seam changed")
	}
	different := 0
	for _, frame := range a.frames {
		if frame.RGBAAt(0, 0).A != 0 {
			t.Fatal("transparent background lost")
		}
		if !bytes.Equal(frame.Pix, a.frames[0].Pix) {
			different++
		}
	}
	if different < 100 {
		t.Fatal("animation frames are identical")
	}
}

func TestGIFDisposalClearsPreviousSprite(t *testing.T) {
	palette := color.Palette{color.Transparent, color.RGBA{R: 255, A: 255}}
	withDot := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	withDot.SetColorIndex(0, 0, 1)
	clear := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	animation := &gif.GIF{Config: image.Config{Width: 2, Height: 2}, Image: []*image.Paletted{withDot, clear}, Delay: []int{3, 3}, Disposal: []byte{gif.DisposalBackground, gif.DisposalNone}}
	decoded := prepareMascot(animation)
	if decoded.frames[0].RGBAAt((spriteW-spriteH)/2, 0).A == 0 || decoded.frames[1].RGBAAt((spriteW-spriteH)/2, 0).A != 0 {
		t.Fatal("prior frame remained visible after disposal")
	}
}

func TestLegacyAppearanceFallsBackAndImport(t *testing.T) {
	saved := clock
	t.Cleanup(func() { clock = saved })
	clock.Mascot = "original"
	if len(activeSprite().frames) < 2 || spriteLabel() != "角色圖示：蜜袋鼯" {
		t.Fatal("legacy preference did not fall back to the animated sugar glider")
	}
	if _, err := resources.ReadFile("art/original_mascot.png"); err == nil {
		t.Fatal("removed character is still embedded")
	}
	raw, err := resources.ReadFile("art/floating_mascot.png")
	if err != nil {
		t.Fatal(err)
	}
	sprite, err := decodeMascot(raw)
	if err != nil || len(sprite.frames) != 1 {
		t.Fatalf("invalid fallback PNG: %v", err)
	}
	if sprite.frames[0].RGBAAt(0, 0).A != 0 {
		t.Fatal("fallback sprite aspect ratio padding lost")
	}
	if _, err = decodeMascot([]byte("invalid image")); err == nil {
		t.Fatal("invalid import accepted")
	}
	if clampSpriteScale(10) != 60 || clampSpriteScale(190) != 175 {
		t.Fatal("failed to cap drag resize")
	}
}

func TestBuiltInFoxAppearance(t *testing.T) {
	saved := clock
	previousKey, previousSprite := cachedAppearance, cachedSprite
	t.Cleanup(func() { clock = saved; cachedAppearance, cachedSprite = previousKey, previousSprite })
	clock.Mascot = "fox"
	a := activeSprite()
	if len(a.frames) != 152 || a.ends[len(a.ends)-1] != 6760*time.Millisecond {
		t.Fatal("incorrect fox animation")
	}
	if spriteLabel() != "角色圖示：可愛狐狸" {
		t.Fatal("missing fox label")
	}
	if !bytes.Equal(a.frames[0].Pix, a.frames[len(a.frames)-1].Pix) {
		t.Fatal("fox animation seam changed")
	}
}
