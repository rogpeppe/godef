package wire
import (
	"container/heap"
	"fmt"
	"reflect"
	"os"
)

type Converter func(dst, src reflect.Value) os.Error

type NewConverter interface {
	NewConverter(dst, src reflect.Type) Converter
}



type naiveConversions struct{}

func (naiveConversions) NewConverter(dst, src reflect.Type) Converter {
	if src == dst {
		return identity
	}
	if src.AssignableTo(dst) {
		return func(dst, src reflect.Value) os.Error {
			dst.Set(src)
			return nil
		}
	}
	return nil
}

type graphConversions struct {
	nodes map[reflect.Type] *node
}

func newGraphConversions() *graphConversions {
	return &graphConversions{make(map[reflect.Type] *node)}
}

// A mark holds data about a node when we are doing
// a shortest path calculation. It is not stored as part
// of a node so that we can do several shortest path
// calculations in parallel if we wish.
type mark struct {
	node *node		// node that this mark is on.
	dist int			// total distance to this node.
	dups int			// count of duplicate path lengths.
	previous *mark		// previous node in the shortest path.
	arc *arc			// arc between here and previous.
	index int			// index into markHeap
}

// node represents one node in the type graph.
// The slice of arcs lead from it other types via
// conversion operators.
type node struct {
	t reflect.Type
	arc []*arc
}

// arc represents a possible conversion between two types.
type arc struct {
	cvt Converter
	dst *node
	name string
}

// find shortest path between two types using Dijstra's algorithm.
func (g *graphConversions) shortestPath(dst, src reflect.Type) *mark {
	n0 := g.nodes[src]
	n1 := g.nodes[dst]
	if n0 == nil || n1 == nil {
		return nil
	}
	q := markHeap{&mark{node: n0, dist: 0}}
	marks := map[*node]*mark{n0: q[0]}
	for len(q) > 0 {
		u := q[0]
		if u.node == n1 {
			return u
		}
		heap.Remove(&q, 0)
		for _, a := range u.node.arc {
			if v := marks[a.dst]; v != nil {
				if u.dist + 1 < v.dist {
					heap.Remove(&q, u.index)
					v.dist = u.dist + 1
					v.dups = 0
					v.arc = a
					heap.Push(&q, u)
				}else if u.dist + 1 == v.dist {
					v.dups++
				}
			}else{
				v := &mark{node: a.dst, dist: u.dist + 1, previous: u, arc: a}
				heap.Push(&q, v)
				marks[a.dst] = v
			}
		}
	}
	return nil
}

// duplicatePath returns a non-nil mark if there
// is there is more than one shortest path between
// any two nodes in the conversion graph.
// The returned mark is one of the duplicate
// paths.
func (g *graphConversions) duplicatePath() *mark {
	// find duplicate path by finding shortest path
	// between all nodes and seeing if any have
	// duplicate lengths.
	for t0 := range g.nodes {
		for t1 := range g.nodes {
			if t0 == t1 {
				continue
			}
			p := g.shortestPath(t1, t0)
			for m := p; m != nil; m = m.previous {
				if m.dups > 0 {
					return p
				}
			}
		}
	}
	return nil
}
			
func (g *graphConversions) NewConverter(dst, src reflect.Type) Converter {
	if src == dst {
		return identity
	}
	m := g.shortestPath(dst, src)
	if m == nil {
		return nil
	}
	fmt.Printf("conversion from %v -> %v\n", src, dst)
	fmt.Printf("\t%s\n", pathString(m))
	for l := m; l.previous != nil; l = l.previous {
		if l.dups > 0 {
			panic("duplicate path!")
		}
	}
	return newConverter(m)
}

func newConverter(m *mark) Converter {
	if m.previous.previous == nil {
		return func(dst, src reflect.Value) os.Error {
			return m.arc.cvt(dst, src)
		}
	}
	cvt := newConverter(m.previous)
	return func(dst, src reflect.Value) os.Error {
		tmp := reflect.New(m.previous.node.t).Elem()
		if err := cvt(tmp, src); err != nil {
			return err
		}
		if err := m.arc.cvt(dst, tmp); err != nil {
			return err
		}
		return nil
	}
}

func pathString(p *mark) string {
	if p.previous == nil {
		return p.node.t.String()
	}
	return pathString(p.previous) + "->(" + p.arc.name + ")->"+ p.node.t.String()
}

// AddConversion adds a new possible conversion from src to dst,
// using cvt. The given name is used for informational purposes only.
// It returns an error if the conversion causes an ambiguity.
func (g *graphConversions) AddConversion(dst, src reflect.Type, cvt Converter, name string) os.Error {
	n0 := g.nodes[src]
	addn0 := n0 == nil
	if addn0 {
		n0 = &node{t: src}
		g.nodes[src] = n0
	}
	n1 := g.nodes[dst]
	addn1 := n1 ==nil
	if addn1 {
		n1 = &node{t: dst}
		g.nodes[dst] = n1
	}
	n0.arc = append(n0.arc, &arc{cvt, n1, name})

	if p1 := g.duplicatePath(); p1 != nil {
		// remove added arc
		n0.arc = n0.arc[0 : len(n0.arc)-1]

		// remove new nodes if they were added
		if addn0 {
			g.nodes[src] = nil, false
		}
		if addn1 {
			g.nodes[dst] = nil, false
		}

		// find start of duplicate path
		p0 := p1
		for p0.previous != nil {
			p0 = p0.previous
		}
		oldp := g.shortestPath(p1.node.t, p0.node.t)
		if oldp == nil {
			panic(fmt.Errorf("duplicate path but no path previously (%v)", p1))
		}
		
		// TODO say what the duplicate path is
		return fmt.Errorf("duplicate of conversion path {%v} when adding %q {%v->%v}", pathString(oldp), name, src, dst)
	}
	return nil
}

// heap of marked nodes, for use with Dijkstra's algorithm.
type markHeap []*mark

func (h *markHeap) Len() int {
	return len(*h)
}

func (h *markHeap) Less(i, j int) bool {
	a := *h
	return a[i].dist < a[j].dist
}

func (h *markHeap) Swap(i, j int) {
	a := *h
	a[i], a[j] = a[j], a[i]
	a[i].index = i
	a[j].index = j
}

func (h *markHeap) Push(x interface{}) {
	m := x.(*mark)
	a := *h
	m.index = len(a)
	*h = append(a, m)
}

func (h *markHeap) Pop() interface{} {
	a := *h
	x := a[len(a)-1]
	*h = a[0:len(a)-1]
	x.index = -1
	return x
}
