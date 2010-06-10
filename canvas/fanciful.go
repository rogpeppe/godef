package canvas

import (
	"exp/draw"
)

type MoveableItem interface {
	Item
	SetCentre(p draw.Point)
}

type dragger struct {
	Item
	it MoveableItem
}

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
