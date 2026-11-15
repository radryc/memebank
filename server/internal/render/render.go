// Package render draws meme captions onto template images on the server. It
// backs the on-demand image endpoint so memes can be stored as text specs.
package render

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	_ "image/gif"
	_ "image/jpeg"

	_ "golang.org/x/image/webp"
)

//go:embed Anton-Regular.ttf
var antonTTF []byte

const maxRenderSide = 1600

// CaptionLayout places one caption block on the canvas. Every value is
// relative to the canvas size so the browser preview and the server render
// stay aligned at any resolution.
type CaptionLayout struct {
	X    float64 `json:"x"`    // centre of the block, 0..1 of canvas width
	Y    float64 `json:"y"`    // vertical centre of the block, 0..1 of canvas height
	Size float64 `json:"size"` // font size as a fraction of canvas width
}

// Style mirrors the JSON stored in memes.style.
type Style struct {
	FontScale float64 `json:"fontScale"`
	Color     string  `json:"color"`
	Stroke    string  `json:"stroke"`
	Uppercase bool    `json:"uppercase"`
	Outline   bool    `json:"outline"`
	// Top and Bottom are optional. When nil the classic top/bottom layout is
	// used; when set, the caption is drawn at the user's position and size.
	Top    *CaptionLayout `json:"top,omitempty"`
	Bottom *CaptionLayout `json:"bottom,omitempty"`
}

const (
	defaultCaptionSize = 0.095
	minCaptionSize     = 0.02
	maxCaptionSize     = 0.30
)

// ParseStyle applies the same defaults as the client renderer.
func ParseStyle(raw string) Style {
	style := Style{
		FontScale: 1,
		Color:     "#ffffff",
		Stroke:    "#000000",
		Uppercase: true,
		Outline:   true,
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &style); err != nil {
			return Style{FontScale: 1, Color: "#ffffff", Stroke: "#000000", Uppercase: true, Outline: true}
		}
	}
	if style.FontScale < 0.4 || style.FontScale > 2.5 || math.IsNaN(style.FontScale) {
		style.FontScale = 1
	}
	if !validHex(style.Color) {
		style.Color = "#ffffff"
	}
	if !validHex(style.Stroke) {
		style.Stroke = "#000000"
	}
	style.Top = sanitizeCaptionLayout(style.Top)
	style.Bottom = sanitizeCaptionLayout(style.Bottom)
	return style
}

// sanitizeCaptionLayout clamps client-supplied positions and sizes so a
// hand-crafted style JSON cannot push text off the canvas or use absurd sizes.
func sanitizeCaptionLayout(layout *CaptionLayout) *CaptionLayout {
	if layout == nil {
		return nil
	}
	if layout.X == 0 && layout.Y == 0 && layout.Size == 0 {
		return nil
	}
	out := *layout
	if math.IsNaN(out.X) || math.IsInf(out.X, 0) {
		out.X = 0.5
	}
	if math.IsNaN(out.Y) || math.IsInf(out.Y, 0) {
		out.Y = 0.5
	}
	if math.IsNaN(out.Size) || math.IsInf(out.Size, 0) || out.Size <= 0 {
		out.Size = defaultCaptionSize
	}
	out.X = clampFloat(out.X, 0, 1)
	out.Y = clampFloat(out.Y, 0, 1)
	out.Size = clampFloat(out.Size, minCaptionSize, maxCaptionSize)
	return &out
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

var (
	fontOnce sync.Once
	anton    *opentype.Font
	fontErr  error
)

func loadFont() (*opentype.Font, error) {
	fontOnce.Do(func() {
		anton, fontErr = opentype.Parse(antonTTF)
	})
	if fontErr != nil {
		return nil, fmt.Errorf("parse embedded font: %w", fontErr)
	}
	return anton, nil
}

func newFace(size float64) (font.Face, error) {
	f, err := loadFont()
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingNone,
	})
}

// RenderFile reads a template image and returns a captioned PNG.
func RenderFile(path, topText, bottomText, rawStyle string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode template: %w", err)
	}
	return Render(img, topText, bottomText, ParseStyle(rawStyle))
}

// Render draws captions onto src and encodes the result as PNG.
func Render(src image.Image, topText, bottomText string, style Style) ([]byte, error) {
	canvas := scaleCanvas(src, maxRenderSide)
	if err := drawCaptions(canvas, topText, bottomText, style); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, canvas); err != nil {
		return nil, fmt.Errorf("encode meme: %w", err)
	}
	return buf.Bytes(), nil
}

func scaleCanvas(src image.Image, maxSide int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := math.Min(1, float64(maxSide)/float64(max(w, h)))
	dw, dh := int(math.Round(float64(w)*scale)), int(math.Round(float64(h)*scale))
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	if dw == w && dh == h {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	}
	return dst
}

// captionBlock is one laid-out block of caption lines.
type captionBlock struct {
	lines      []string
	size       float64
	lineHeight float64
	centerX    float64
	topY       float64
	height     float64
}

func drawCaptions(canvas *image.RGBA, topText, bottomText string, style Style) error {
	upper := func(s string) string {
		if style.Uppercase {
			return strings.ToUpper(s)
		}
		return s
	}
	top := strings.TrimSpace(upper(topText))
	bottom := strings.TrimSpace(upper(bottomText))
	if top == "" && bottom == "" {
		return nil
	}

	width := float64(canvas.Bounds().Dx())
	height := float64(canvas.Bounds().Dy())
	const lineHeightRatio = 1.08
	maxWidth := width * 0.92
	pad := height * 0.028

	base := width * defaultCaptionSize * style.FontScale
	autoSize := fitCaptionSize(width, height, base, top, bottom, style)

	fill := color.NRGBA(parseColor(style.Color))
	stroke := color.NRGBA(parseColor(style.Stroke))

	for _, spec := range []struct {
		text   string
		layout *CaptionLayout
		isTop  bool
	}{
		{top, style.Top, true},
		{bottom, style.Bottom, false},
	} {
		if spec.text == "" {
			continue
		}
		size := autoSize
		if spec.layout != nil {
			size = spec.layout.Size * width
		}
		block, err := newCaptionBlock(spec.text, width, height, spec.layout, size, maxWidth, pad, lineHeightRatio, spec.isTop)
		if err != nil {
			return err
		}
		if err := drawCaptionBlock(canvas, block, fill, stroke, style.Outline); err != nil {
			return err
		}
	}
	return nil
}

// fitCaptionSize mirrors the browser auto-fit loop. Captions that have an
// explicit layout opt out: their size belongs to the user.
func fitCaptionSize(width, height, base float64, topText, bottomText string, style Style) float64 {
	maxWidth := width * 0.92
	minSize := math.Max(12, width*0.032)
	const lineHeightRatio = 1.08
	size := math.Max(minSize, base)
	for guard := 0; guard < 40; guard++ {
		face, err := newFace(size)
		if err != nil {
			return size
		}
		fits := true
		check := func(text string, layout *CaptionLayout) {
			if layout != nil || text == "" {
				return
			}
			lines := wrapText(text, face, maxWidth)
			if float64(len(lines))*size*lineHeightRatio > height*0.42 {
				fits = false
				return
			}
			for _, line := range lines {
				if measure(face, line) > width*0.94 {
					fits = false
					return
				}
			}
		}
		check(topText, style.Top)
		check(bottomText, style.Bottom)
		closeFace(face)
		if fits {
			return size
		}
		if size <= minSize {
			return size
		}
		size = math.Max(minSize, size-math.Max(1, math.Round(size*0.055)))
	}
	return size
}

func newCaptionBlock(
	text string,
	width, height float64,
	layout *CaptionLayout,
	size, maxWidth, pad, lineHeightRatio float64,
	isTop bool,
) (*captionBlock, error) {
	face, err := newFace(size)
	if err != nil {
		return nil, err
	}
	defer closeFace(face)

	lines := wrapText(text, face, maxWidth)
	widest := 0.0
	for _, line := range lines {
		if w := measure(face, line); w > widest {
			widest = w
		}
	}
	blockHeight := float64(len(lines)) * size * lineHeightRatio
	block := &captionBlock{
		lines:      lines,
		size:       size,
		lineHeight: size * lineHeightRatio,
		height:     blockHeight,
	}
	if layout != nil {
		block.centerX, block.topY = clampCaptionBox(layout.X*width, layout.Y*height, widest, blockHeight, width, height)
		return block, nil
	}
	block.centerX = width / 2
	block.topY = pad
	if !isTop {
		block.topY = height - pad - blockHeight
	}
	return block, nil
}

// clampCaptionBox keeps a positioned block inside the canvas. It accepts the
// block centre and returns the clamped horizontal centre plus the block top.
func clampCaptionBox(centerX, centerY, blockWidth, blockHeight, width, height float64) (float64, float64) {
	halfW := blockWidth / 2
	if blockWidth >= width {
		centerX = width / 2
	} else {
		centerX = clampFloat(centerX, halfW, width-halfW)
	}
	halfH := blockHeight / 2
	if blockHeight >= height {
		centerY = blockHeight / 2
	} else {
		centerY = clampFloat(centerY, halfH, height-halfH)
	}
	return centerX, centerY - halfH
}

func drawCaptionBlock(canvas *image.RGBA, block *captionBlock, fill, stroke color.NRGBA, outline bool) error {
	face, err := newFace(block.size)
	if err != nil {
		return err
	}
	defer closeFace(face)
	// Canvas positions text from the line box top, while Go's font ascent is
	// much taller for Anton. This offset keeps server renders aligned with the
	// live editor preview.
	firstBaseline := block.topY + block.size*0.78
	return drawBlock(canvas, face, block.lines, block.centerX, firstBaseline, block.lineHeight, fill, stroke, outline, block.size)
}

func closeFace(face font.Face) {
	if closer, ok := face.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

func drawBlock(
	canvas *image.RGBA,
	face font.Face,
	lines []string,
	centerX, firstBaseline, lineHeight float64,
	fill, stroke color.NRGBA,
	outline bool,
	size float64,
) error {
	strokeSrc := image.NewUniform(stroke)
	fillSrc := image.NewUniform(fill)
	radius := math.Max(1, size*0.06)
	const directions = 16

	for i, line := range lines {
		if line == "" {
			continue
		}
		x := centerX - measure(face, line)/2
		y := firstBaseline + float64(i)*lineHeight
		if outline && stroke.A > 0 {
			for d := 0; d < directions; d++ {
				angle := 2 * math.Pi * float64(d) / directions
				dx := math.Cos(angle) * radius
				dy := math.Sin(angle) * radius
				drawer := font.Drawer{
					Dst:  canvas,
					Src:  strokeSrc,
					Face: face,
					Dot:  fixed.P(int(math.Round(x+dx)), int(math.Round(y+dy))),
				}
				drawer.DrawString(line)
			}
		}
		drawer := font.Drawer{
			Dst:  canvas,
			Src:  fillSrc,
			Face: face,
			Dot:  fixed.P(int(math.Round(x)), int(math.Round(y))),
		}
		drawer.DrawString(line)
	}
	return nil
}

func wrapText(text string, face font.Face, maxWidth float64) []string {
	if text == "" {
		return nil
	}
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := words[0]
		for _, word := range words[1:] {
			candidate := line + " " + word
			if measure(face, candidate) <= maxWidth {
				line = candidate
			} else {
				lines = append(lines, line)
				line = word
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func measure(face font.Face, text string) float64 {
	return float64(font.MeasureString(face, text)) / 64
}

func validHex(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, c := range value[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func parseColor(value string) color.RGBA {
	if !validHex(value) {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	var r, g, b uint8
	fmt.Sscanf(value[1:], "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
