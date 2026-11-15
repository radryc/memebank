package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestRenderAddsCaptions(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 600, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 20, G: 40, B: 80, A: 255})
		}
	}
	out, err := Render(src, "when the build passes", "on the first try", ParseStyle("{}"))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 400 {
		t.Fatalf("size = %v", img.Bounds())
	}

	// Captions are drawn near the top and bottom edges; look for bright pixels
	// that were not part of the dark source image.
	brightTop, brightBottom := 0, 0
	for y := 0; y < 80; y++ {
		for x := 0; x < 600; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 0xff00 && g > 0xff00 && b > 0xff00 {
				brightTop++
			}
		}
	}
	for y := 320; y < 400; y++ {
		for x := 0; x < 600; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 0xff00 && g > 0xff00 && b > 0xff00 {
				brightBottom++
			}
		}
	}
	if brightTop == 0 || brightBottom == 0 {
		t.Fatalf("captions missing: top=%d bottom=%d", brightTop, brightBottom)
	}
}

func TestRenderHonorsCaptionLayout(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	// A red caption centred on (100,100) at 12% of the canvas width.
	out, err := Render(src, "HI", "", ParseStyle(`{"color":"#ff0000","outline":false,"top":{"x":0.25,"y":0.25,"size":0.12}}`))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	red := 0
	for y := 50; y < 150; y++ {
		for x := 50; x < 150; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r > 0xf000 && g < 0x3000 && b < 0x3000 {
				red++
			}
		}
	}
	if red == 0 {
		t.Fatal("positioned caption was not drawn at its requested location")
	}

	// The default top edge should stay untouched because the caption moved.
	for y := 0; y < 40; y++ {
		for x := 0; x < 400; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r < 0xf000 || g < 0xf000 || b < 0xf000 {
				t.Fatalf("caption leaked into the top edge at (%d,%d)", x, y)
			}
		}
	}
}

func TestStyleCaptionLayoutClamping(t *testing.T) {
	if got := ParseStyle(`{}`).Top; got != nil {
		t.Fatalf("default layout = %+v, want nil", got)
	}
	if got := ParseStyle(`{"bottom":{"x":0,"y":0,"size":0}}`).Bottom; got != nil {
		t.Fatalf("empty layout = %+v, want nil", got)
	}

	style := ParseStyle(`{"top":{"x":-2,"y":5,"size":4}}`)
	if style.Top == nil {
		t.Fatal("expected a top layout")
	}
	if style.Top.X != 0 || style.Top.Y != 1 || style.Top.Size != maxCaptionSize {
		t.Fatalf("clamped layout = %+v", style.Top)
	}

	custom := ParseStyle(`{"bottom":{"x":0.5,"y":0.25,"size":0.1}}`)
	if custom.Bottom == nil || custom.Bottom.X != 0.5 || custom.Bottom.Y != 0.25 || custom.Bottom.Size != 0.1 {
		t.Fatalf("custom layout = %+v", custom.Bottom)
	}
}

func TestStyleDefaults(t *testing.T) {
	style := ParseStyle("")
	if style.FontScale != 1 || style.Color != "#ffffff" || !style.Uppercase || !style.Outline {
		t.Fatalf("defaults = %+v", style)
	}
	custom := ParseStyle(`{"fontScale":1.25,"color":"#c8ff4d","stroke":"#000000","uppercase":false,"outline":false}`)
	if custom.FontScale != 1.25 || custom.Color != "#c8ff4d" || custom.Uppercase || custom.Outline {
		t.Fatalf("custom = %+v", custom)
	}
	if got := ParseStyle(`{"color":"nope"}`).Color; got != "#ffffff" {
		t.Fatalf("invalid color = %q", got)
	}
}
