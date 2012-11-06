package main

import (
	"code.google.com/p/freetype-go/freetype/truetype"
	"code.google.com/p/rog-go/canvas"
	"code.google.com/p/rog-go/values"
	"code.google.com/p/x-go-binding/ui"
	"code.google.com/p/x-go-binding/ui/x11"
	"fmt"
	"image"
	"image/color"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"os"
	"time"
)

// to add:
// modifications for mac os X11
// should it crash if Draw is passed a non-canonical rectangle?
// it's a pity that image.RGBAColor isn't a image.Color

type line struct {
	obj    *canvas.Line
	p0, p1 image.Point
}

type realPoint struct {
	x, y float64
}

type ball struct {
	p   realPoint
	v   realPoint
	col color.Color
}

type lineList struct {
	line line
	next *lineList
}

var currtime int64

const updateTime = 0.01e9

var window *canvas.Canvas
var lines *lineList
var lineVersion int

var sleepTime = 10 * time.Millisecond

const ballSize = 8

var red = color.RGBA{0xff, 0, 0, 0xff}
var blue = color.RGBA{0, 0, 0xff, 0xff}

func main() {
	rand.Seed(0)
	ctxt, err := x11.NewWindow()
	if ctxt == nil {
		log.Fatalf("no window: %v", err)
	}
	screen := ctxt.Screen()
	bg := canvas.NewBackground(screen.(*image.RGBA), image.White, flushFunc(ctxt))
	window = canvas.NewCanvas(nil, bg.Rect())
	bg.SetItem(window)
	nballs := 0
	ctxt.FlushImage()

	csz := window.Rect().Max

	// add edges of window
	addLine(image.Pt(-1, -1), image.Pt(csz.X, -1))
	addLine(image.Pt(csz.X, -1), image.Pt(csz.X, csz.Y))
	addLine(image.Pt(csz.X, csz.Y), image.Pt(-1, csz.Y))
	addLine(image.Pt(-1, csz.Y), image.Pt(-1, -1))

	go sliderProc()

	makeRect(image.Rect(30, 30, 200, 100), setAlpha(red, 128))
	makeRect(image.Rect(150, 90, 230, 230), setAlpha(blue, 128))

	window.Flush()

	mkball := make(chan ball)
	delball := make(chan bool)
	pause := make(chan bool)

	go monitor(mkball, delball, pause)

	for i := 0; i < nballs; i++ {
		mkball <- randBall()
	}
	ecc := make(chan (<-chan interface{}))
	ec := ctxt.EventChan()
	for {
		select {
		case e, ok := <-ec:
			if !ok {
				return
			}
			switch e := e.(type) {
			case ui.MouseEvent:
				if e.Buttons == 0 {
					break
				}
				if window.HandleMouse(window, e, ec) {
					break
				}
				switch {
				case e.Buttons&1 != 0:
					go handleMouse(e, ec, ecc, lineMaker)
					ec = nil
				case e.Buttons&2 != 0:
					go handleMouse(e, ec, ecc, func(m ui.MouseEvent, ec <-chan interface{}) {
						ballMaker(e, ec, mkball)
					})
					ec = nil
				case e.Buttons&4 != 0:
					delball <- true
				}
			case ui.KeyEvent:
				fmt.Printf("got key %c (%d)\n", e.Key, e.Key)
				switch e.Key {
				case ' ':
					pause <- true
				case 'd':
					delball <- true
				}
			default:
				fmt.Printf("unknown event %v\n", e)
			}
		case ec = <-ecc:
			break
		}
	}
}

// this will go.
type RectFlusherContext interface {
	ui.Window
	FlushImageRect(r image.Rectangle)
}

func flushFunc(ctxt ui.Window) func(r image.Rectangle) {
	if fctxt, ok := ctxt.(RectFlusherContext); ok {
		return func(r image.Rectangle) {
			fctxt.FlushImageRect(r)
		}
	}
	return func(_ image.Rectangle) {
		ctxt.FlushImage()
	}
}

func sliderProc() {
	val := values.NewValue(float64(0.0), nil)
	window.AddItem(canvas.NewSlider(image.Rect(10, 10, 100, 40), image.White, blue, val))
	window.AddItem(canvas.NewSlider(image.Rect(15, 35, 100, 70), image.White, setAlpha(red, 128), val))
	window.Flush()
	rval := values.Transform(val, values.UnitFloat64ToRangedFloat64(0.001e9, 0.1e9))
	timeText := canvas.NewText(
		image.Pt(10, 80), canvas.N|canvas.W, "", defaultFont(), 12, values.Transform(rval, values.Float64Multiply(1e-6).Combine(values.Float64ToString("%6.2gms", "%gms"))))
	window.AddItem(timeText)
	g := rval.Getter()
	for {
		x, ok := g.Get()
		if !ok {
			break
		}
		sleepTime = time.Duration(x.(float64))
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

// Start a modal loop to handle mouse events, running f.
// f is passed the mouse event that caused the modal loop
// to be started, and the mouse channel.
// When f finishes, the mouse channel is handed back
// on ecc.
func handleMouse(m ui.MouseEvent, ec <-chan interface{}, ecc chan (<-chan interface{}), f func(first ui.MouseEvent, ec <-chan interface{})) {
	defer func() {
		ecc <- ec
	}()
	f(m, ec)
}

func randBall() ball {
	csz := window.Rect().Max
	var b ball
	b.p = randPoint(csz)
	b.v.x = rand.Float64() - 0.5
	b.v.y = rand.Float64() - 0.5
	if b.v.x == 0 && b.v.y == 0 {
		panic("did that really happen?!")
	}
	b.v, _ = makeUnit(b.v)
	speed := 0.1e-6 + rand.Float64()*0.4e-6
	b.v.x *= speed
	b.v.y *= speed
	b.col = randColour()
	return b
}

func randPoint(size image.Point) realPoint {
	return realPoint{
		rand.Float64() * float64(size.X-1),
		rand.Float64() * float64(size.Y-1),
	}
}

func randColour() (c color.RGBA) {
	v := uint32(rand.Int63()<<8) | 0x808080ff
	return color.RGBA{
		R: uint8(v >> 24),
		G: uint8(v >> 16),
		B: uint8(v >> 8),
		A: 0xff,
	}
}

func addLine(p0, p1 image.Point) *line {
	obj := canvas.NewLine(image.Black, p0, p1, 3)
	window.AddItem(obj)
	ln := line{obj, p0, p1}
	lines = &lineList{ln, lines}
	lineVersion++
	return &lines.line
}

func (p realPoint) point() image.Point {
	return image.Point{round(p.x), round(p.y)}
}

func getMouse(ec <-chan interface{}) (ui.MouseEvent, bool) {
	for {
		switch e := (<-ec).(type) {
		case ui.MouseEvent:
			return e, true
		case nil:
			return ui.MouseEvent{}, false
		default:
			fmt.Printf("discard %v", e)
		}
	}
	panic("not reached")
}

func lineMaker(m ui.MouseEvent, ec <-chan interface{}) {
	m0 := m
	ln := addLine(m0.Loc, m0.Loc)
	for m.Buttons&1 != 0 {
		var ok bool
		m, ok = getMouse(ec)
		if !ok {
			return
		}
		ln.obj.SetEndPoints(m0.Loc, m.Loc)
		ln.p1 = m.Loc
		lineVersion++
		window.Flush()
	}
}

func ballMaker(m ui.MouseEvent, ec <-chan interface{}, mkball chan<- ball) {
	const sampleTime = 250 * time.Millisecond
	var vecs [8]realPoint // approx sampleTime's worth of velocities
	i := 0
	n := 0
	m0 := m
	m1 := m
	for {
		var ok bool
		m1, ok = getMouse(ec)
		if !ok {
			break
		}
		dt := m1.Time.Sub(m.Time)
		if dt >= sampleTime/time.Duration(len(vecs)) || m.Buttons&2 == 0 {
			delta := draw2realPoint(m1.Loc.Sub(m.Loc))
			vecs[i].x = delta.x / float64(dt)
			vecs[i].y = delta.y / float64(dt)
			i = (i + 1) % len(vecs)
			if n < len(vecs) {
				n++
			}
			m = m1
		}
		if m.Buttons&2 == 0 {
			break
		}
	}
	var avg realPoint
	for _, v := range vecs {
		avg.x += v.x
		avg.y += v.y
	}
	avg.x /= float64(n)
	avg.y /= float64(n)
	var b ball
	speed := math.Sqrt(avg.x*avg.x + avg.y*avg.y) // in pixels/ns
	if speed < 10e-9 {
		// a click with no drag starts a ball with random velocity.
		b = randBall()
		b.p = draw2realPoint(m0.Loc)
	} else {
		v, _ := makeUnit(draw2realPoint(m1.Loc.Sub(m0.Loc)))
		v.x *= speed
		v.y *= speed
		b = ball{
			realPoint{float64(m0.Loc.X), float64(m0.Loc.Y)},
			v,
			randColour(),
		}
	}
	mkball <- b
}

func draw2realPoint(p image.Point) realPoint {
	return realPoint{float64(p.X), float64(p.Y)}
}

func makeRect(r image.Rectangle, col color.Color) {
	img := canvas.Box(r.Dx(), r.Dy(), image.NewUniform(col), 1, image.Black)
	item := canvas.NewImage(img, opaqueColor(col), r.Min)
	window.AddItem(canvas.Draggable(item))
}

func opaqueColor(col color.Color) bool {
	_, _, _, a := col.RGBA()
	return a == 0xffff
}

func monitor(mkball <-chan ball, delball <-chan bool, pause <-chan bool) {
	ballcountText := canvas.NewText(
		image.Pt(window.Rect().Max.X-5, 5), canvas.N|canvas.E, "0 balls", defaultFont(), 30, nil)
	window.AddItem(canvas.Draggable(ballcountText))
	ballcountText.SetFill(image.NewUniform(red))
	window.Flush()
	ctl := make(chan (chan<- bool))
	nballs := 0
	for {
		select {
		case b := <-mkball:
			go animateBall(ctl, b)
			nballs++
			ballcountText.SetText(fmt.Sprintf("%d balls", nballs))
			window.Flush()

		case <-pause:
			reply := make(chan bool)
			for i := 0; i < nballs; i++ {
				ctl <- reply
			}
			<-pause
			for i := 0; i < nballs; i++ {
				<-reply
			}
		case <-delball:
			// delete a random ball
			if nballs > 0 {
				ctl <- nil
				nballs--
				ballcountText.SetText(fmt.Sprintf("%d balls", nballs))
				window.Flush()
			}
		}
	}
}

type Ball struct {
	item *canvas.Image
}

func makeBall(b ball) Ball {
	img := canvas.Box(ballSize, ballSize, image.NewUniform(b.col), 1, image.Black)
	p := b.p.point().Sub(image.Pt(ballSize/2, ballSize/2))
	item := canvas.NewImage(img, true, p)
	window.AddItem(item)
	window.Raise(item, nil, false)
	return Ball{item}
}

func (obj *Ball) SetCentre(p realPoint) {
	obj.item.SetCentre(image.Point{round(p.x), round(p.y)})
}

const large = 1000000

func animateBall(c <-chan (chan<- bool), b ball) {
	var speed float64
	b.v, speed = makeUnit(b.v)
	obj := makeBall(b)
	var hitline line
	smallcount := 0
	version := lineVersion
loop:
	for {
		var hitp realPoint
		dist := float64(large)
		oldline := hitline
		for l := lines; l != nil; l = l.next {
			ln := l.line
			ok, hp, hdist := intersect(b.p, b.v, ln)
			if ok && hdist < dist && ln.obj != oldline.obj && (smallcount < 10 || hdist > 1.5) {
				hitp, hitline, dist = hp, ln, hdist
			}
		}
		if dist == large {
			fmt.Printf("no intersection!\n")
			window.Delete(obj.item)
			window.Flush()
			for {
				reply := <-c
				if reply == nil {
					return
				}
				reply <- false
			}
		}
		if dist < 1e-4 {
			smallcount++
		} else {
			smallcount = 0
		}
		bouncev := boing(b.v, hitline)
		t0 := time.Now()
		dt := time.Duration(dist / speed)
		t := time.Duration(0)
		for {
			s := float64(t) * speed
			currp := realPoint{b.p.x + s*b.v.x, b.p.y + s*b.v.y}
			obj.SetCentre(currp)
			window.Flush()
			if lineVersion > version {
				b.p, hitline, version = currp, oldline, lineVersion
				continue loop
			}
			select {
			case reply := <-c:
				if reply == nil {
					window.Delete(obj.item)
					window.Flush()
					return
				}
				reply <- false
				// we were paused, so pretend no time went by
				t0 = time.Now().Add(-t)
			default:
			}
			time.Sleep(sleepTime)
			t = time.Now().Sub(t0)
			if t >= dt {
				break
			}
		}
		b.p = hitp
		b.v = bouncev
	}
}

// makeUnit makes a vector of unit-length parallel to v.
func makeUnit(v realPoint) (realPoint, float64) {
	mag := math.Sqrt(v.x*v.x + v.y*v.y)
	return realPoint{v.x / mag, v.y / mag}, mag
}

// bounce ball travelling in direction av off line b.
// return the new unit vector.
func boing(av realPoint, ln line) realPoint {
	f := ln.p1.Sub(ln.p0)
	d := math.Atan2(float64(f.Y), float64(f.X))*2 - math.Atan2(av.y, av.x)
	p := realPoint{math.Cos(d), math.Sin(d)}

	return p
}

// compute the intersection of lines a and b.
// b is assumed to be fixed, and a is indefinitely long
// but doesn't extend backwards from its starting point.
// a is defined by the starting point p and the unit vector v.
func intersect(p, v realPoint, b line) (ok bool, pt realPoint, dist float64) {
	const zero = 1e-6

	w := realPoint{float64(b.p1.X - b.p0.X), float64(b.p1.Y - b.p0.Y)}
	det := w.x*v.y - v.x*w.y
	if det > -zero && det < zero {
		return
	}

	y21 := float64(b.p0.Y) - p.y
	x21 := float64(b.p0.X) - p.x
	dist = (w.x*y21 - w.y*x21) / det
	if dist < 0.0 {
		return
	}

	pt = realPoint{p.x + v.x*dist, p.y + v.y*dist}
	if b.p0.X > b.p1.X {
		b.p0.X, b.p1.X = b.p1.X, b.p0.X
	}
	if b.p0.Y > b.p1.Y {
		b.p0.Y, b.p1.Y = b.p1.Y, b.p0.Y
	}

	ok = round(pt.x) >= b.p0.X &&
		round(pt.x) <= b.p1.X &&
		round(pt.y) >= b.p0.Y &&
		round(pt.y) <= b.p1.Y
	return
}

func round(x float64) int {
	if x < 0 {
		x -= 0.5
	} else {
		x += 0.5
	}
	return int(x)
}

func mult(v, alpha, oldAlpha uint8) uint8 {
	nv := int(v) * int(alpha) / int(oldAlpha)
	if nv < 0 {
		return 0
	}
	if nv > 0xff {
		return 0xff
	}
	return uint8(nv)
}

func setAlpha(c color.RGBA, a uint8) color.RGBA {
	if c.A == 0 {
		return c
	}
	c.R = mult(c.R, a, c.A)
	c.G = mult(c.G, a, c.A)
	c.B = mult(c.B, a, c.A)
	c.A = a
	return c
}
