package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"sort"
	"sync"
	"time"
)

const (
	spriteW = 236
	spriteH = 150
	spriteX = 52
	spriteY = 144
)

type mascotAnimation struct {
	frames []*image.RGBA
	ends   []time.Duration
}

var mascotFrames = sync.OnceValue(func() mascotAnimation {
	data, err := resources.ReadFile("art/floating_mascot.gif")
	if err != nil {
		return mascotAnimation{}
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return mascotAnimation{}
	}
	return prepareMascot(decoded)
})

func prepareMascot(animation *gif.GIF) mascotAnimation {
	var ready mascotAnimation
	full := image.NewRGBA(image.Rect(0, 0, animation.Config.Width, animation.Config.Height))
	var end time.Duration
	for i, part := range animation.Image {
		disposal := byte(gif.DisposalNone)
		if i < len(animation.Disposal) {
			disposal = animation.Disposal[i]
		}
		var previous *image.RGBA
		if disposal == gif.DisposalPrevious {
			previous = image.NewRGBA(full.Bounds())
			copy(previous.Pix, full.Pix)
		}
		draw.Draw(full, part.Bounds(), part, part.Bounds().Min, draw.Over)
		ready.frames = append(ready.frames, placeMascot(full))
		end += time.Duration(max(2, animation.Delay[i])) * 10 * time.Millisecond
		ready.ends = append(ready.ends, end)
		if disposal == gif.DisposalBackground {
			draw.Draw(full, part.Bounds(), image.NewUniform(color.Transparent), image.Point{}, draw.Src)
		}
		if previous != nil {
			full = previous
		}
	}
	return ready
}
func (a mascotAnimation) index(elapsed time.Duration) int {
	if len(a.ends) == 0 {
		return -1
	}
	position := max(0, elapsed) % a.ends[len(a.ends)-1]
	return sort.Search(len(a.ends), func(i int) bool { return a.ends[i] > position })
}

func placeMascot(source image.Image) *image.RGBA {
	bounds := source.Bounds()
	scale := min(float64(spriteW)/float64(bounds.Dx()), float64(spriteH)/float64(bounds.Dy()))
	width, height := max(1, int(float64(bounds.Dx())*scale+.5)), max(1, int(float64(bounds.Dy())*scale+.5))
	resized := resizeRGBA(source, width, height)
	canvas := image.NewRGBA(image.Rect(0, 0, spriteW, spriteH))
	draw.Draw(canvas, resized.Bounds().Add(image.Pt((spriteW-width)/2, spriteH-height)), resized, image.Point{}, draw.Src)
	return canvas
}

func resizeRGBA(source image.Image, width, height int) *image.RGBA {
	bounds := source.Bounds()
	input, ok := source.(*image.RGBA)
	if !ok {
		input = image.NewRGBA(bounds)
		draw.Draw(input, bounds, source, bounds.Min, draw.Src)
	}
	output := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy := float64(y) * float64(bounds.Dy()-1) / float64(max(1, height-1))
		y0 := int(sy)
		y1 := min(y0+1, bounds.Dy()-1)
		dy := sy - float64(y0)
		for x := 0; x < width; x++ {
			sx := float64(x) * float64(bounds.Dx()-1) / float64(max(1, width-1))
			x0 := int(sx)
			x1 := min(x0+1, bounds.Dx()-1)
			dx := sx - float64(x0)
			off := output.PixOffset(x, y)
			a, b, c, d := input.PixOffset(bounds.Min.X+x0, bounds.Min.Y+y0), input.PixOffset(bounds.Min.X+x1, bounds.Min.Y+y0), input.PixOffset(bounds.Min.X+x0, bounds.Min.Y+y1), input.PixOffset(bounds.Min.X+x1, bounds.Min.Y+y1)
			for i := 0; i < 4; i++ {
				output.Pix[off+i] = byte((1-dy)*((1-dx)*float64(input.Pix[a+i])+dx*float64(input.Pix[b+i])) + dy*((1-dx)*float64(input.Pix[c+i])+dx*float64(input.Pix[d+i])) + .5)
			}
		}
	}
	return output
}
