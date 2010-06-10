package canvas

import (
	"exp/draw"
)

// A MoveableItem is an item that may be
// moved by calling SetCentre, where the
// centre is the central point of the item's
// bounding box.
//
type MoveableItem interface {
	Item
	SetCentre(p draw.Point)
}

type dragger struct {
	Item
	it MoveableItem
}

// Draggable makes any MoveableItem into
// an object that may be dragged by the
// mouse.
//
func Draggable(it MoveableItem) Item {
	return &dragger{it, it}
}

var _ HandlerItem = &dragger{}

func (d *dragger) HandleMouse(f Flusher, m draw.Mouse, mc <-chan draw.Mouse) bool {
	if m.Buttons&1 == 0 {
		return false
	}
	delta := centre(d.it.Bbox()).Sub(m.Point)
	but := m.Buttons
	for {
		m = <-mc
		d.it.SetCentre(m.Add(delta))
		f.Flush()
		if (m.Buttons & but) != but {
			break
		}
	}
	return true
}
