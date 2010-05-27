package canvas

import (
	"rog-go.googlecode.com/hg/draw"
	"image"
	"freetype-go.googlecode.com/hg/freetype/raster"
)


// A RasterItem is a low level canvas object that
// can be used to build higher level primitives.
// It implements Item, and will calculate
// (and remember) its bounding box on request.
//
// Otherwise it can be used as a raster.Rasterizer.
//
type RasterItem struct {
	raster.Rasterizer
	bbox draw.Rectangle
	raster.RGBAPainter
	clipper clippedPainter
}

// CalcBbox calculates the current bounding box of
// all the pixels in the current path.
func (obj *RasterItem) CalcBbox() {
	obj.bbox = rasterBbox(&obj.Rasterizer)
}

func (obj *RasterItem) Draw(dst *image.RGBA, clipr draw.Rectangle) {
	obj.clipper.Clipr = clipr
	obj.Image = dst
	obj.clipper.Painter = &obj.RGBAPainter
	obj.Rasterize(&obj.clipper)
}

func (obj *RasterItem) HitTest(p draw.Point) bool {
	var hit hitTestPainter
	hit.P = p
	obj.Rasterize(&hit)
	return hit.Hit
}

func (obj *RasterItem) SetContainer(_ *Canvas) {
}

func (obj *RasterItem) Bbox() draw.Rectangle {
	return obj.bbox
}

func (obj *RasterItem) Opaque() bool {
	return false
}

type clippedPainter struct {
	Painter raster.Painter
	Clipr   draw.Rectangle
}

func (p *clippedPainter) Paint(ss []raster.Span, last bool) {
	r := p.Clipr
	j := 0

	// quick check that we've at least got some rows that might be painted
	if len(ss) > 0 && ss[0].Y < r.Max.Y && ss[len(ss)-1].Y > r.Min.Y {
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
	P   draw.Point
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

type checkPainter struct {
	Painter   *raster.RGBAPainter
	PreCheck  func(c image.RGBAColor, p draw.Point) bool
	PostCheck func(c image.RGBAColor, p draw.Point) bool
	Ok        bool
}

func (p *checkPainter) Paint(ss []raster.Span, last bool) {
	for _, s := range ss {
		row := p.Painter.Image.Pixel[s.Y]
		if p.PreCheck != nil {
			for x := s.X0; x < s.X1; x++ {
				if !p.PreCheck(row[x], draw.Point{x, s.Y}) {
					p.Ok = false
				}
			}
		}
	}
	p.Painter.Paint(ss, last)
	for _, s := range ss {
		row := p.Painter.Image.Pixel[s.Y]
		if p.PostCheck != nil {
			for x := s.X0; x < s.X1; x++ {
				if !p.PostCheck(row[x], draw.Point{x, s.Y}) {
					p.Ok = false
				}
			}
		}
	}
}


// A bboxPainter is a raster.Painter that calculates
// the bounding box of all spans that it is asked to paint.
// Each Paint request will be forwarded
// to Painter if it is non-nil.
type bboxPainter struct {
	R       draw.Rectangle
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
		p.R = draw.ZR
	} else {
		p.R = r
	}
}

func rasterBbox(rasterizer *raster.Rasterizer) (r draw.Rectangle) {
	var bbox bboxPainter
	rasterizer.Rasterize(&bbox)
	return bbox.R
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
	fixScale = 1 << fixBits // matches raster.Fixed
)

func float2fix(f float) raster.Fixed {
	return raster.Fixed(f*fixScale + 0.5)
}

func int2fix(i int) raster.Fixed {
	return raster.Fixed(i << fixBits)
}

func fix2int(i raster.Fixed) int {
	return int((i + fixScale/2) >> fixBits)
}

func pixel2fixPoint(p draw.Point) raster.Point {
	return raster.Point{raster.Fixed(p.X << fixBits), raster.Fixed(p.Y << fixBits)}
}

func fix2pixelPoint(p raster.Point) draw.Point {
	return draw.Point{int((p.X + fixScale/2) >> fixBits), int((p.Y + fixScale/2) >> fixBits)}
}

func float2fixed(f float64) raster.Fixed {
	if f < 0 {
		return raster.Fixed(f*256 + 0.5)
	}
	return raster.Fixed(f*256 - 0.5)
}

func fixed2float(f raster.Fixed) float64 {
	return float64(f) / 256
}
