package main

import (
	"rog-go.googlecode.com/hg/x11"
	"exp/draw"
	"image"
	"io/ioutil"
	"fmt"
	"log"
	"math"
	"os"
	"rand"
	"time"
	"rog-go.googlecode.com/hg/canvas"
	"freetype-go.googlecode.com/hg/freetype/truetype"
)

// to add:
// Rectangle.Eq()
// modifications for mac os X11
// should it crash if Draw is passed a non-canonical rectangle?
// it's a pity that image.RGBAColor isn't a draw.Color

// os.Error("heelo") gives internal compiler error

type RectFlusherContext interface {
	draw.Context
	FlushImageRect(r draw.Rectangle)
}

type line struct {
	obj    *canvas.Line
	p0, p1 draw.Point
}

type realPoint struct {
	x, y float64
}

type ball struct {
	p   realPoint
	v   realPoint
	col draw.Color
}

type lineList struct {
	line line
	next *lineList
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

var currtime int64

const updateTime = 0.01e9

var window *canvas.Canvas
var lines *lineList
var lineVersion int

var sleepTime = int64(0.01e9)

const ballSize = 8

func main() {
	rand.Seed(0)
	ctxt, err := x11.NewWindow()
	if ctxt == nil {
		log.Exitf("no window: %v", err)
	}
	screen := ctxt.Screen()
	bg := canvas.NewBackground(screen.(*image.RGBA), draw.White, flushFunc(ctxt))
	window = canvas.NewCanvas(bg, nil, draw.Rect(0, 0, bg.Width(), bg.Height()))
	bg.SetItem(window)
	nballs := 0
	ctxt.FlushImage()

	csz := draw.Pt(window.Width(), window.Height())

	// add edges of window
	addLine(draw.Pt(-1, -1), draw.Pt(csz.X, -1))
	addLine(draw.Pt(csz.X, -1), draw.Pt(csz.X, csz.Y))
	addLine(draw.Pt(csz.X, csz.Y), draw.Pt(-1, csz.Y))
	addLine(draw.Pt(-1, csz.Y), draw.Pt(-1, -1))

	go sliderProc()

	makeRect(draw.Rect(30, 30, 200, 100), draw.Red.SetAlpha(128))
	makeRect(draw.Rect(150, 90, 230, 230), draw.Blue.SetAlpha(128))

	window.Flush()

	mkball := make(chan ball)
	delball := make(chan bool)
	pause := make(chan bool)

	go monitor(mkball, delball, pause)

	for i := 0; i < nballs; i++ {
		mkball <- randBall()
	}
	mc := ctxt.MouseChan()
	mcc := make(chan (<-chan draw.Mouse))
	qc := ctxt.QuitChan()
	kc := ctxt.KeyboardChan()
	for {
		select {
		case <-qc:
			fmt.Printf("quitting\n")
			return
		case m := <-mc:
			if m.Buttons == 0 {
				break
			}
			if window.HandleMouse(window, m, mc) {
				break
			}
			switch {
			case m.Buttons&4 != 0:
				return
			case m.Buttons&1 != 0:
				go handleMouse(m, mc, mcc, lineMaker)
				mc = nil
			case m.Buttons&2 != 0:
				go handleMouse(m, mc, mcc, func(m draw.Mouse, mc <-chan draw.Mouse) {
					ballMaker(m, mc, mkball)
				})
				mc = nil
			}
		case k := <-kc:
			fmt.Printf("got key %c (%d)\n", k, k)
			switch k {
			case ' ':
				pause <- true
			case 'd':
				delball <- true
			}
		case mc = <-mcc:
			break
		}
	}
}

func sliderProc() {
	val := canvas.NewValue(float64(0.0))
	window.AddItem(canvas.NewSlider(draw.Rect(10, 10, 100, 40), draw.White, draw.Blue, val))
	window.AddItem(canvas.NewSlider(draw.Rect(15, 35, 100, 70), draw.White, draw.Red.SetAlpha(128), val))
	window.Flush()
	rval := canvas.Transform(val, canvas.UnitFloat2RangedFloat(0.001e9, 0.1e9))
	timeText := canvas.NewText(
		draw.Pt(10, 80), canvas.N|canvas.W, "", defaultFont(), 12, canvas.Transform(rval, canvas.FloatMultiply(1e-6).Combine(canvas.Float2String("%6.2gms", "%gms"))))
	window.AddItem(timeText)
	for x := range rval.Iter() {
		sleepTime = int64(x.(float64))
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


// Start a modal loop to handle mouse events, running f.
// f is passed the mouse event that caused the modal loop
// to be started, and the mouse channel.
// When f finishes, the mouse channel is handed back
// on mcc.
func handleMouse(m draw.Mouse, mc <-chan draw.Mouse, mcc chan (<-chan draw.Mouse), f func(first draw.Mouse, mc <-chan draw.Mouse)) {
	defer func() {
		mcc <- mc
	}()
	f(m, mc)
}

func randBall() ball {
	csz := draw.Point{window.Width(), window.Height()}
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

func randPoint(size draw.Point) realPoint {
	return realPoint{
		rand.Float64() * float64(size.X-1),
		rand.Float64() * float64(size.Y-1),
	}
}

func randColour() (c draw.Color) {
	return draw.Color(uint32(rand.Int63()<<8) | 0x808080ff)
}

func addLine(p0, p1 draw.Point) *line {
	obj := canvas.NewLine(image.Black, p0, p1, 3)
	window.AddItem(obj)
	ln := line{obj, p0, p1}
	lines = &lineList{ln, lines}
	lineVersion++
	return &lines.line
}

func (p realPoint) point() draw.Point {
	return draw.Point{round(p.x), round(p.y)}
}

func lineMaker(m draw.Mouse, mc <-chan draw.Mouse) {
	p0 := m.Point
	ln := addLine(p0, p0)
	for m.Buttons&1 != 0 {
		m = <-mc
		ln.obj.SetEndPoints(p0, m.Point)
		ln.p1 = m.Point
		lineVersion++
		window.Flush()
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func ballMaker(m draw.Mouse, mc <-chan draw.Mouse, mkball chan<- ball) {
	const sampleTime = 0.25e9
	var vecs [8]realPoint // approx sampleTime's worth of velocities
	i := 0
	n := 0
	m0 := m
	m1 := m
	for {
		m1 = <-mc
		dt := m1.Nsec - m.Nsec
		if dt >= sampleTime/int64(len(vecs)) || m.Buttons&2 == 0 {
			delta := draw2realPoint(m1.Sub(m.Point))
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
		b.p = draw2realPoint(m0.Point)
	} else {
		v, _ := makeUnit(draw2realPoint(m1.Sub(m0.Point)))
		v.x *= speed
		v.y *= speed
		b = ball{
			realPoint{float64(m0.X), float64(m0.Y)},
			v,
			randColour(),
		}
	}
	mkball <- b
}

func draw2realPoint(p draw.Point) realPoint {
	return realPoint{float64(p.X), float64(p.Y)}
}

func makeRect(r draw.Rectangle, col draw.Color) {
	img := canvas.Box(r.Dx(), r.Dy(), col, 1, image.Black)
	item := canvas.NewImage(img, opaqueColor(col), r.Min)
	window.AddItem(canvas.Draggable(item))
}

func opaqueColor(col image.Color) bool {
	_, _, _, a := col.RGBA()
	return a == 0xffff
}

func monitor(mkball <-chan ball, delball <-chan bool, pause <-chan bool) {
	ballcountText := canvas.NewText(
		draw.Pt(window.Width()-5, 5), canvas.N|canvas.E, "0 balls", defaultFont(), 30, nil)
	window.AddItem(canvas.Draggable(ballcountText))
	ballcountText.SetColor(image.Red)
	window.Flush()
	ctl := make(chan (chan<- bool))
	nballs := 0
	for {
		select {
		case b := <-mkball:
			go animateBall(ctl, b)
			nballs++
			ballcountText.SetText(fmt.Sprintf("%d balls", nballs))

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
			}
		}
	}
}

type Ball struct {
	*canvas.Image
}

func makeBall(b ball) Ball {
	img := canvas.Box(ballSize, ballSize, b.col, 1, image.Black)
	p := b.p.point().Sub(draw.Pt(ballSize/2, ballSize/2))
	item := canvas.NewImage(img, true, p)
	window.AddItem(item)
	window.Lower(item, nil)
	return Ball{item}
}

func (obj *Ball) Move(p realPoint) {
	bp := draw.Point{round(p.x), round(p.y)}.Sub(draw.Pt(ballSize/2, ballSize/2))
	obj.Image.SetMinPoint(bp)
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
			window.Delete(obj)
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
		t0 := time.Nanoseconds()
		dt := int64(dist / speed)
		t := int64(0)
		for {
			s := float64(t) * speed
			currp := realPoint{b.p.x + s*b.v.x, b.p.y + s*b.v.y}
			obj.Move(currp)
			window.Flush()
			if lineVersion > version {
				b.p, hitline, version = currp, oldline, lineVersion
				continue loop
			}
			if reply, ok := <-c; ok {
				if reply == nil {
					window.Delete(obj)
					fmt.Printf("deleted ball\n")
					window.Flush()
					return
				}
				reply <- false
				// we were paused, so pretend no time went by
				t0 = time.Nanoseconds() - t
			}
			time.Sleep(sleepTime)
			t = time.Nanoseconds() - t0
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
