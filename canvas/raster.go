package canvas

import (
	"code.google.com/p/freetype-go/freetype/raster"
	"fmt"
	"image"
	"image/color"
	"image/draw"
)

// A RasterItem is a low level canvas object that
// can be used to build higher level primitives.
// It implements Item, and will calculate
// (and remember) its bounding box on request.
//
// Otherwise it can be used as a raster.Rasterizer.
//
type RasterItem struct {
	rasterizer raster.Rasterizer
	fill       image.Image
	bbox       image.Rectangle
	clipper    clippedPainter
}

// CalcBbox calculates the current bounding box of
// all the pixels in the current path.
func (obj *RasterItem) CalcBbox() {
	obj.bbox = rasterBbox(&obj.rasterizer)
}

var yellow = image.Uniform{color.RGBA{0xff, 0xdd, 0xdd, 0xff}}

func (obj *RasterItem) Draw(dst draw.Image, clipr image.Rectangle) {
	obj.clipper.Clipr = clipr
	obj.clipper.Painter = NewPainter(dst, obj.fill, draw.Over)
	//fmt.Printf("drawing, bbox %v, clipped to %v\n", obj.bbox, clipr)
	//draw.Draw(dst, clipr, yellow, clipr.Min)
	obj.rasterizer.Rasterize(&obj.clipper)
}

func (obj *RasterItem) SetFill(fill image.Image) {
	obj.fill = fill
}

func (obj *RasterItem) HitTest(p image.Point) bool {
	var hit hitTestPainter
	hit.P = p
	obj.rasterizer.Rasterize(&hit)
	return hit.Hit
}

func (obj *RasterItem) SetContainer(b Backing) {
	r := b.Rect()
	obj.rasterizer.Dx = r.Min.X
	obj.rasterizer.Dy = r.Min.Y
	obj.rasterizer.SetBounds(r.Dx(), r.Dy())
	obj.Clear()
}

func (obj *RasterItem) pt(p raster.Point) raster.Point {
	return raster.Point{p.X + raster.Fix32(obj.rasterizer.Dx)<<fixBits, p.Y + raster.Fix32(obj.rasterizer.Dy)<<fixBits}
}

func (obj *RasterItem) Add1(p raster.Point) {
	obj.rasterizer.Add1(obj.pt(p))
}

func (obj *RasterItem) Add2(p0, p1 raster.Point) {
	obj.rasterizer.Add2(obj.pt(p0), obj.pt(p1))
}

func (obj *RasterItem) Add3(p0, p1, p2 raster.Point) {
	obj.rasterizer.Add3(obj.pt(p0), obj.pt(p1), obj.pt(p2))
}

func (obj *RasterItem) Start(p raster.Point) {
	obj.rasterizer.Start(p)
}

func (obj *RasterItem) Clear() {
	obj.rasterizer.Clear()
}

func (obj *RasterItem) Bbox() image.Rectangle {
	fmt.Printf("raster bbox %v\n", obj.bbox)
	return obj.bbox
}

func (obj *RasterItem) Opaque() bool {
	return false
}

type clippedPainter struct {
	Painter raster.Painter
	Clipr   image.Rectangle
}

func (p *clippedPainter) Paint(ss []raster.Span, last bool) {
	r := p.Clipr
	j := 0

	// quick check that we've at least got some rows that might be painted
	if len(ss) > 0 && ss[0].Y < r.Max.Y && ss[len(ss)-1].Y >= r.Min.Y {
		for i, s := range ss {
			if s.Y >= r.Min.Y {
				ss = ss[i:]
				break
			}
		}
		for i, s := range ss {
			if s.Y >= r.Max.Y {
				break
			}
			if s.X0 < r.Max.X && r.Min.X < s.X1 {
				sp := &ss[j]
				if i != j {
					*sp = s
				}
				if s.X0 < r.Min.X {
					sp.X0 = r.Min.X
				}
				if s.X1 > r.Max.X {
					sp.X1 = r.Max.X
				}
				j++
			}
		}
	}
	if j > 0 || last {
		p.Painter.Paint(ss[0:j], last)
	}
}

type hitTestPainter struct {
	P   image.Point
	Hit bool
}

func (h *hitTestPainter) Paint(ss []raster.Span, _ bool) {
	p := h.P
	if len(ss) > 0 && p.Y >= ss[0].Y && p.Y <= ss[len(ss)-1].Y {
		for i, s := range ss {
			if s.Y == p.Y {
				ss = ss[i:]
				break
			}
		}
		for _, s := range ss {
			if s.Y != p.Y {
				break
			}
			if p.X >= s.X0 && p.X < s.X1 {
				h.Hit = true
				// If we were feeling Evil, we could do a panic
				// to return control without painting any
				// more spans. But probably best to keep
				// the run time constant for interactive use.
				return
			}
		}
	}
}

//type checkPainter struct {
//	Painter   *raster.RGBAPainter
//	PreCheck  func(c color.RGBA, p image.Point) bool
//	PostCheck func(c color.RGBA, p image.Point) bool
//	Ok        bool
//}
//
//func (p *checkPainter) Paint(ss []raster.Span, last bool) {
//	img := p.Painter.Image
//	for _, s := range ss {
//		row := img.Pix[s.Y*img.Stride:]
//		if p.PreCheck != nil {
//			for x := s.X0; x < s.X1; x++ {
//				if !p.PreCheck(row[x], image.Point{x, s.Y}) {
//					p.Ok = false
//				}
//			}
//		}
//	}
//	p.Painter.Paint(ss, last)
//	for _, s := range ss {
//		row := img.Pix[s.Y*img.Stride:]
//		if p.PostCheck != nil {
//			for x := s.X0; x < s.X1; x++ {
//				if !p.PostCheck(row[x], image.Point{x, s.Y}) {
//					p.Ok = false
//				}
//			}
//		}
//	}
//}

// A bboxPainter is a raster.Painter that calculates
// the bounding box of all spans that it is asked to paint.
// Each Paint request will be forwarded
// to Painter if it is non-nil.
type bboxPainter struct {
	R       image.Rectangle
	Painter raster.Painter
}

func (p *bboxPainter) Paint(ss []raster.Span, last bool) {
	r := p.R
	if r.Min.X >= r.Max.X || r.Min.Y >= r.Max.Y {
		r.Min.X = 0x7fffffff
		r.Min.Y = 0x7fffffff
		r.Max.X = -0x8000000
		r.Max.Y = -0x8000000
	}
	if len(ss) > 0 {
		sp := &ss[0]
		if sp.Y < r.Min.Y {
			r.Min.Y = sp.Y
		}
		sp = &ss[len(ss)-1]
		if sp.Y+1 > r.Max.Y {
			r.Max.Y = sp.Y + 1
		}
		for i := range ss {
			sp := &ss[i]
			if sp.X0 < r.Min.X {
				r.Min.X = sp.X0
			}
			if sp.X1 > r.Max.X {
				r.Max.X = sp.X1
			}
		}
	}
	if p.Painter != nil {
		p.Painter.Paint(ss, last)
	}
	if r.Min.X > r.Max.X || r.Min.Y > r.Max.Y {
		p.R = image.ZR
	} else {
		p.R = r
	}
}

func rasterBbox(rasterizer *raster.Rasterizer) (r image.Rectangle) {
	var bbox bboxPainter
	rasterizer.Rasterize(&bbox)
	return bbox.R
}

// slow but comprehensive.
type genericImagePainter struct {
	image draw.Image
	src   image.Image
	op    draw.Op
}

func SpanBbox(ss []raster.Span) image.Rectangle {
	if len(ss) == 0 {
		return image.ZR
	}
	r := image.Rect(1000000, ss[0].Y, -1000000, ss[len(ss)-1].Y+1)
	for _, s := range ss {
		if s.X0 < r.Min.X {
			r.Min.X = s.X0
		}
		if s.X1 > r.Max.X {
			r.Max.X = s.X1
		}
	}
	return r
}

func alphaColorImage(alpha uint16) *image.Uniform {
	return &image.Uniform{color.Alpha16{uint16(alpha)}}
}

func (g *genericImagePainter) Paint(ss []raster.Span, done bool) {
	for _, s := range ss {
		draw.DrawMask(g.image,
			image.Rect(s.X0, s.Y, s.X1, s.Y+1),
			g.src,
			image.Point{s.X0, s.X1},
			alphaColorImage(uint16(s.A)),
			image.ZP,
			g.op)
	}
}

// NewPainter returns a Painter that will draw from src onto
// dst using the Porter-Duff composition operator op.
func NewPainter(dst draw.Image, src image.Image, op draw.Op) (p raster.Painter) {
	defer func() { fmt.Printf("newpainter %T\n", p) }()
	if src, ok := src.(*image.Uniform); ok {
		switch dst := dst.(type) {
		case *image.Alpha:
			if _, _, _, a := src.RGBA(); a == 0xffff {
				switch op {
				case draw.Src:
					return &raster.AlphaSrcPainter{dst}
				case draw.Over:
					return &raster.AlphaOverPainter{dst}
				}
			}

		case *image.RGBA:
			p := raster.NewRGBAPainter(dst)
			p.SetColor(src)
			p.Op = op
			return p
		}
	}
	return &genericImagePainter{dst, src, op}
}

func spans2ys(ss []raster.Span) []int {
	f := make([]int, len(ss))
	for i, s := range ss {
		f[i] = s.Y
	}
	return f
}

const (
	fixBits  = 8
	fixScale = 1 << fixBits // matches raster.Fix32
)

func float2fix(f float64) raster.Fix32 {
	return raster.Fix32(f*fixScale + 0.5)
}

func int2fix(i int) raster.Fix32 {
	return raster.Fix32(i << fixBits)
}

func fix2int(i raster.Fix32) int {
	return int((i + fixScale/2) >> fixBits)
}

func pixel2fixPoint(p image.Point) raster.Point {
	return raster.Point{raster.Fix32(p.X << fixBits), raster.Fix32(p.Y << fixBits)}
}

func fix2pixelPoint(p raster.Point) image.Point {
	return image.Point{int((p.X + fixScale/2) >> fixBits), int((p.Y + fixScale/2) >> fixBits)}
}

func float2fixed(f float64) raster.Fix32 {
	if f < 0 {
		return raster.Fix32(f*256 + 0.5)
	}
	return raster.Fix32(f*256 - 0.5)
}

func fixed2float(f raster.Fix32) float64 {
	return float64(f) / 256
}
