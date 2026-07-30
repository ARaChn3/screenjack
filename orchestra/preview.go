package main

import (
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"
	"time"

	"github.com/qeesung/image2ascii/convert"
)

// PreviewModel handles ASCII art preview of images/GIFs
type PreviewModel struct {
	frames    []string // ASCII frames
	delays    []int    // Frame delays in ms (for GIFs)
	current   int
	width     int
	height    int
	path      string
	err       error
	isGif     bool
	lastFrame time.Time
}

// NewPreview creates a preview for the given asset
func NewPreview(assetPath string, width, height int) PreviewModel {
	p := PreviewModel{
		width:  width,
		height: height,
		path:   assetPath,
	}
	p.load()
	return p
}

func (p *PreviewModel) load() {
	fullPath := "../assets/" + p.path

	f, err := os.Open(fullPath)
	if err != nil {
		p.err = err
		return
	}
	defer f.Close()

	// Check if GIF
	if strings.HasSuffix(strings.ToLower(p.path), ".gif") {
		p.loadGif(fullPath)
	} else {
		p.loadStatic(fullPath)
	}
}

func (p *PreviewModel) loadGif(path string) {
	f, err := os.Open(path)
	if err != nil {
		p.err = err
		return
	}
	defer f.Close()

	g, err := gif.DecodeAll(f)
	if err != nil {
		p.err = err
		return
	}

	p.isGif = true
	converter := convert.NewImageConverter()
	opts := convert.DefaultOptions
	opts.FixedWidth = p.width
	opts.FixedHeight = p.height
	opts.Colored = true

	// Build composite frames (GIF frames are often partial)
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	canvas := image.NewRGBA(bounds)

	for i, frame := range g.Image {
		// Draw frame onto canvas
		for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
			for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
				canvas.Set(x, y, frame.At(x, y))
			}
		}

		ascii := converter.Image2ASCIIString(canvas, &opts)
		p.frames = append(p.frames, ascii)

		delay := g.Delay[i] * 10 // centiseconds to ms
		if delay == 0 {
			delay = 100
		}
		p.delays = append(p.delays, delay)
	}

	if len(p.frames) == 0 {
		p.err = err
	}
}

func (p *PreviewModel) loadStatic(path string) {
	f, err := os.Open(path)
	if err != nil {
		p.err = err
		return
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		p.err = err
		return
	}

	converter := convert.NewImageConverter()
	opts := convert.DefaultOptions
	opts.FixedWidth = p.width
	opts.FixedHeight = p.height
	opts.Colored = true

	ascii := converter.Image2ASCIIString(img, &opts)
	p.frames = []string{ascii}
	p.delays = []int{0}
}

// Tick advances to next frame if enough time has passed
func (p *PreviewModel) Tick() bool {
	if !p.isGif || len(p.frames) <= 1 {
		return false
	}

	delay := time.Duration(p.delays[p.current]) * time.Millisecond
	if time.Since(p.lastFrame) >= delay {
		p.current = (p.current + 1) % len(p.frames)
		p.lastFrame = time.Now()
		return true
	}
	return false
}

// View returns current ASCII frame
func (p *PreviewModel) View() string {
	if p.err != nil {
		return "error: " + p.err.Error()
	}
	if len(p.frames) == 0 {
		return "loading..."
	}
	return p.frames[p.current]
}

// IsAnimated returns true if this is a multi-frame GIF
func (p *PreviewModel) IsAnimated() bool {
	return p.isGif && len(p.frames) > 1
}
