// A "simple" program to display some text and let the
// user drag it around. It will get simpler...
package main

import (
	"rog-go.googlecode.com/hg/x11"
	"exp/draw"
	"log"
	"image"
	"io/ioutil"
	"os"
	"rog-go.googlecode.com/hg/canvas"
	"freetype-go.googlecode.com/hg/freetype/truetype"
)

var cvs *canvas.Canvas

func main() {
	ctxt, err := x11.NewWindow()
	if ctxt == nil {
		log.Exitf("no window: %v", err)
	}
	screen := ctxt.Screen()

	bg := canvas.NewBackground(screen.(*image.RGBA), draw.White, flushFunc(ctxt))
	cvs = canvas.NewCanvas(nil, bg.Rect())
	bg.SetItem(cvs)

	item := canvas.Draggable(canvas.Moveable(
		canvas.NewText(
			draw.ZP,
			0,
			"Hello, world",
			defaultFont(),
			30,
			nil)))
	item.SetCentre(draw.Pt(cvs.Rect().Dx()/2, cvs.Rect().Dy()/3))
	cvs.AddItem(item)
//	txtitem :=	canvas.NewText(
//			draw.Pt(100, 100),
//			0,
//			"Working?",
//			defaultFont(),
//			20,
//			nil)


//	img := canvas.ImageOf(txtitem)

//	cvs.AddItem(canvas.NewImage(img, false, draw.Pt(cvs.Width() / 2, cvs.Height()*2/3)))

	qc := ctxt.QuitChan()
	kc := ctxt.KeyboardChan()
	mc := ctxt.MouseChan()
	cvs.Flush()

	for {
		select {
		case <-qc:
			log.Exit("quitting")
			return
		case m := <-mc:
			if m.Buttons == 0 {
				break
			}
			cvs.HandleMouse(cvs, m, mc)
		case <-kc:
		}
	}
}

func defaultFont() *truetype.Font {
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		log.Exit("no goroot set")
	}
	path := goroot + "/src/pkg/freetype-go.googlecode.com/hg/luxi-fonts/luxisr.ttf"
	// Read the font data.
	fontBytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Exit(err)
	}
	font, err := truetype.Parse(fontBytes)
	if err != nil {
		log.Exit(err)
	}
	return font
}

// this will go.
type RectFlusherContext interface {
	draw.Context
	FlushImageRect(r draw.Rectangle)
}
func flushFunc(ctxt draw.Context) func(r draw.Rectangle) {
	if fctxt, ok := ctxt.(RectFlusherContext); ok {
		return func(r draw.Rectangle) {
			fctxt.FlushImageRect(r)
		}
	}
	return func(_ draw.Rectangle) {
		ctxt.FlushImage()
	}
}
