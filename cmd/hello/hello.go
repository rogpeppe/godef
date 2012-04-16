// A "simple" program to display some text and let the
// user drag it around. It will get simpler...
package main

import (
	"code.google.com/p/freetype-go/freetype/truetype"
	"code.google.com/p/rog-go/canvas"
	"code.google.com/p/rog-go/x11"
	"exp/draw"
	"image"
	"io/ioutil"
	"log"
	"os"
)

var cvs *canvas.Canvas

func main() {
	win, err := x11.NewWindow()
	if win == nil {
		log.Fatalf("no window: %v", err)
	}
	screen := win.Screen()

	bg := canvas.NewBackground(screen.(*image.RGBA), image.White, flushFunc(win))
	cvs = canvas.NewCanvas(nil, bg.Rect())
	bg.SetItem(cvs)

	item := canvas.Draggable(canvas.Moveable(
		canvas.NewText(
			image.ZP,
			0,
			"Hello, world",
			defaultFont(),
			30,
			nil)))
	item.SetCentre(image.Pt(cvs.Rect().Dx()/2, cvs.Rect().Dy()/3))
	cvs.AddItem(item)
	//	txtitem :=	canvas.NewText(
	//			image.Pt(100, 100),
	//			0,
	//			"Working?",
	//			defaultFont(),
	//			20,
	//			nil)

	//	img := canvas.ImageOf(txtitem)

	//	cvs.AddItem(canvas.NewImage(img, false, image.Pt(cvs.Width() / 2, cvs.Height()*2/3)))

	cvs.Flush()
	ec := win.EventChan()
	for {
		switch e := (<-ec).(type) {
		case nil:
			log.Fatal("quitting")
			return
		case draw.MouseEvent:
			if e.Buttons == 0 {
				break
			}
			cvs.HandleMouse(cvs, e, ec)
		}
	}
}

func filterMouseEvents(ec <-chan interface{}, mc chan<- draw.MouseEvent) {
	for e := range ec {
		if e, ok := e.(draw.MouseEvent); ok {
			mc <- e
		}
	}
}

func defaultFont() *truetype.Font {
	goroot := os.Getenv("GOROOT")
	if goroot == "" {
		log.Fatal("no goroot set")
	}
	path := goroot + "/src/pkg/freetype-go.googlecode.com/hg/luxi-fonts/luxisr.ttf"
	// Read the font data.
	fontBytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	font, err := truetype.Parse(fontBytes)
	if err != nil {
		log.Fatal(err)
	}
	return font
}

// this will go.
type RectFlusherContext interface {
	draw.Window
	FlushImageRect(r image.Rectangle)
}

func flushFunc(ctxt draw.Window) func(r image.Rectangle) {
	if fctxt, ok := ctxt.(RectFlusherContext); ok {
		return func(r image.Rectangle) {
			fctxt.FlushImageRect(r)
		}
	}
	return func(_ image.Rectangle) {
		ctxt.FlushImage()
	}
}
