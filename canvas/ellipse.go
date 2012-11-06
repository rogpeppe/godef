package canvas

import (
	"code.google.com/p/freetype-go/freetype/raster"
	"image"
)

// A ellipse object represents an ellipse centered in cr
// with radiuses ra and rb
type Ellipse struct {
	Item
	raster  RasterItem
	backing Backing
	cr      raster.Point
	ra, rb  raster.Fix32
	width   raster.Fix32
	pts     pointVec
}

type pointVec []raster.Point

func (v *pointVec) Push(p raster.Point) {
	*v = append(*v, p)
}

func (v *pointVec) Pop() raster.Point {
	s := *v
	if len(s) == 0 {
		panic("empty vec")
	}
	p := s[len(s)-1]
	*v = s[0 : len(s)-1]
	return p
}

//color, center, radius a radius b and width
func NewEllipse(col image.Image, cr image.Point, ra, rb int, width float64) *Ellipse {
	obj := new(Ellipse)
	obj.cr = pixel2fixPoint(cr)
	obj.ra = int2fix(ra)
	obj.rb = int2fix(rb)
	obj.width = float2fix(width)
	obj.raster.SetFill(col)
	obj.Item = &obj.raster
	obj.pts = nil
	obj.makeOutline()
	obj.backing = NullBacking()
	return obj
}

func (obj *Ellipse) SetContainer(b Backing) {
	obj.backing = b
	obj.raster.SetContainer(b)
	obj.makeOutline()
}

// See A Fast Bresenham Type Algorithm For Drawing Ellipses
// by John Kennedy
func bresham(ra int, sqa, sqb int, pts *pointVec, rev bool) int {
	i := 0
	stopx := 2 * sqb * ra
	stopy := 0

	x := ra
	y := 0

	dx := sqb * (1 - 2*ra)
	dy := sqa

	err := 0

	for stopx >= stopy {
		if rev == false {
			pts.Push(raster.Point{int2fix(x), int2fix(y)})
		} else {
			pts.Push(raster.Point{int2fix(y), int2fix(x)})
		}
		i++
		y++
		stopy += 2 * sqa
		err += dy
		dy += 2 * sqa
		if 2*err+dx > 0 {
			x--
			stopx -= 2 * sqb
			err += dx
			dx += 2 * sqb
		}
	}
	return i
}

// See A Fast Bresenham Type Algorithm For Drawing Ellipses
// by John Kennedy
func (obj *Ellipse) makeOutline() {

	var totnq int
	var pt raster.Point
	var pts2 pointVec

	obj.raster.Clear()
	nquadr2 := 0
	pts := obj.pts
	if len(pts) == 0 {
		sqa := fix2int(obj.ra * obj.ra)
		sqb := fix2int(obj.rb * obj.rb)

		ra := fix2int(obj.ra)
		nquadr := bresham(ra, sqa, sqb, &pts, false)

		rb := fix2int(obj.rb)
		nquadr2 = bresham(rb, sqb, sqa, &pts2, true)
		totnq = nquadr + nquadr2

		obj.pts = pts
	} else {
		totnq = len(pts)
	}
	pt = pts[0]
	pt0 := raster.Point{obj.cr.X + pt.X, obj.cr.Y + pt.Y}
	obj.raster.Start(pt0)
	for j := 0; j < totnq; j++ {
		if nquadr2 > 0 {
			pts.Push(pts2.Pop())
			nquadr2--
		}

		pt = pts[j]
		ptl := raster.Point{obj.cr.X + pt.X, obj.cr.Y + pt.Y}
		obj.raster.Add1(ptl)
	}

	for j := 0; j < totnq; j++ {
		pt = pts[totnq-j-1]
		ptl := raster.Point{obj.cr.X - pt.X, obj.cr.Y + pt.Y}
		obj.raster.Add1(ptl)
	}
	for j := 0; j < totnq; j++ {
		pt = pts[j]
		ptl := raster.Point{obj.cr.X + pt.X, obj.cr.Y - pt.Y}
		obj.raster.Add1(ptl)
	}
	for j := 0; j < totnq; j++ {
		pt = pts[totnq-j-1]
		ptl := raster.Point{obj.cr.X - pt.X, obj.cr.Y - pt.Y}
		obj.raster.Add1(ptl)
	}

	obj.raster.CalcBbox()
}

func (obj *Ellipse) Move(delta image.Point) {
	cr := fix2pixelPoint(obj.cr)
	obj.SetCentre(cr.Add(delta))
}

// SetEndPoints changes the center of the ellipse
//
func (obj *Ellipse) SetCentre(p image.Point) {
	obj.backing.Atomically(func(flush FlushFunc) {
		r := obj.raster.Bbox()
		obj.cr = pixel2fixPoint(p)
		obj.makeOutline()
		flush(r, nil)
		flush(obj.raster.Bbox(), nil)
	})
}

// SetColor changes the colour of the ellipse
//
func (obj *Ellipse) SetFill(fill image.Image) {
	obj.backing.Atomically(func(flush FlushFunc) {
		obj.raster.SetFill(fill)
		flush(obj.raster.Bbox(), nil)
	})
}
