package canvas

import (
	"rog-go.googlecode.com/hg/draw"
)

type MoveableItem interface {
	Item
	Move(delta draw.Point)
}

type dragger struct {
	Item
	it MoveableItem
}

func Draggable(it MoveableItem) Item {
	return dragger{it, it}
}

var _ HandlerItem = dragger{}

func (d dragger) HandleMouse(f Flusher, m draw.Mouse, mc <-chan draw.Mouse) bool {
	if m.Buttons&1 == 0 {
		return false
	}
	p := m.Point
	but := m.Buttons
	for {
		m = <-mc
		p = m.Sub(p)
		d.it.Move(p)
		f.Flush()
		p = m.Point
		if (m.Buttons & but) != but {
			break
		}
	}
	return true
}
