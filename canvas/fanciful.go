package canvas
import (
	"exp/draw"
)


type Dragger interface {
	Move(delta draw.Point)
}

func Drag(obj Dragger, m draw.Mouse, mc <-chan draw.Mouse) {
	p := m.Point
	but := m.Buttons
	for {
		m = <-mc
		p = m.Sub(p)
		obj.Move(p)
		p = m.Point
		if (m.Buttons & but) != but {
			break
		}
	}
}
