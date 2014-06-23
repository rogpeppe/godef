package main
import (
	"fmt"
	"math/rand"
//	"time"
	stats "github.com/patrick-higgins/summstat"
)

func main() {
//	rand.Seed(int64(time.Now().Nanosecond()))
	r := make([]*stats.Stats, 4)
	for i := range r {
		r[i] = stats.NewStats()
//		r[i].CreateBins(100, 0, 200)
	}
	const n = 100000
	for i := 0; i < n; i++ {
		cs := []*character{
			newCharacter(jemimaPaths),
			newCharacter(peterPaths),
			newCharacter(squirrelPaths),
			newCharacter(jeremyPaths),
		}
		for i, c := range cs {
			r[i].AddSample(stats.Sample(c.play()))
		}
	}
	for i, s := range r {
		fmt.Printf("%d. min %g max %g mean %.2g±%.2g median %g\n", i, s.Min(), s.Max(), s.Mean(), s.Stddev(), s.Median())
	}
}

func roll() int {
	return rand.Intn(6) + 1
}

type square struct {
	halt bool
	join func(ch *character) pos
	land func(ch *character)
}

type character struct {
	checkMove func(d int) bool
	backwards bool
	pos pos
	turn int
	extraRolls int
	roll func() int
	paths map[string] []square
}

type pos struct {
	path []square
	i int
}


func (ch *character) play() int {
	for {
		d := ch.roll()
		ch.turn++
		logf("turn %d when starting", ch.turn)
		logf("roll %d", d)
		if d == 6 {
			ch.turn--
			break
		}
	}
	for !ch.playTurn() {
	}
	return ch.turn
}

// turn plays one turn and returns whether the
// game has finished.
func (ch *character) playTurn() bool {
	ch.turn++
	logf("turn %d from %d %p", ch.turn, ch.pos.i, &ch.pos.path[ch.pos.i])
	d := 6
	moved := false
	for d == 6 || ch.extraRolls > 0 {
		if d != 6 {
			ch.extraRolls--
		}
		d = ch.roll()
		logf("roll %d", d)
		if ch.checkMove != nil && !ch.checkMove(d) {
			logf("can't move")
			continue
		}
		moved = true
		if ch.backwards {
			if ch.pos.i <= d {
				logf("bingo!")
				ch.pos.i = 0
				return true
			}
			logf("back %d spaces from %d", d, ch.pos.i)
			ch.pos.i -= d
		} else {
			for i := 0; i < d; i++ {
				ch.pos.i++
				logf("advance 1 to %d", ch.pos.i)
				if ch.pos.atEnd() {
					// got to end - need exact roll
					used := i + 1
					if used == d {
						return true
					}
					ch.pos.i -= used
					logf("failed to finish with exact roll (used %d)", used)
					break
				}
				if s := ch.pos.here(); s.join != nil {
					ch.pos = s.join(ch)
					logf("join %d", ch.pos.i)
				}
				if s := ch.pos.here(); s.halt {
					logf("halt")
					break
				}
			}
		}
	}
	for moved && !ch.backwards {
		s := ch.pos.here()
		if s.land != nil {
			s.land(ch)
		}
		if ch.pos.atEnd() {
			return true
		}
		moved = ch.pos.here() != s
	}
	return false
}

func (p pos) atEnd() bool {
	return p.i >= len(p.path)
}

func (p pos) here() *square {
	return &p.path[p.i]
}

func logf(f string, a ...interface{}) {
//	log.Printf(f, a...)
}

func findPos(ch *character, where string, i int) pos {
	path := ch.paths[where]
	if path == nil || i > len(path) {
		panic(fmt.Errorf("bad location %s:%d in %v", where, i, ch.paths))
	}
	return pos{
		path: path,
		i: i,
	}
}

func joinPos(where string, i int) func(*character) pos {
	return func(ch *character) pos {
		return findPos(ch, where, i)
	}
}

func jumpTo(where string, i int) func(*character) {
	return func(ch *character) {
		logf("jump to %s:%d", where, i)
		ch.pos = findPos(ch, where, i )
	}
}

func combine(fns ...func(ch *character)) func(ch *character) {
	return func(ch *character) {
		for _, f := range fns {
			f(ch)
		}
	}
}

func extraRolls(n int) func(*character) {
	return func(ch *character) {
		logf("%d extra rolls", n)
		ch.extraRolls += n
	}
}

func evenOddAlternate(until int) func(*character) {
	return func(ch *character) {
		logf("even-odd alternate")
		needEven := true
		ch.checkMove = func(d int) bool {
			if ch.pos.i >= until {
				ch.checkMove = nil
				return true
			}
			ok := (d % 2 == 0) == needEven
			if ok {
				needEven = !needEven
			}
			return ok
		}
	}
}

func oddUntil(space int)  func(*character) {
	return until(space, isOdd)
}

func until(space int, check func(d int) bool) func(*character) {
	return func(ch *character) {
		ch.checkMove = func(d int) bool {
			if ch.pos.i >= space {
				ch.checkMove = nil
				return true
			}
			return check(d)
		}
	}
	return nil
}

func forTurns(n int, check func(d int) bool) func(ch *character) {
	return func(ch *character) {
		t := ch.turn + n
		ch.checkMove = func(d int) bool {
			if ch.turn > t {
				ch.checkMove = nil
				return true
			}
			return check(d)
		}
	}
}

func untilSatisfied(n int, check func(d int) bool) func(*character) {
	return func(ch *character) {
		ch.checkMove = func(d int) bool {
			ok := check(d)
			if ok {
				if n--; n <= 0 {
					ch.checkMove = nil
				}
			}
			return ok
		}
	}
	return nil
}

func isEven(d int) bool {
	return d % 2 == 0
}

func isOdd(d int) bool {
	return d % 2 != 0
}

func isOneOf(xs ...int) func(d int) bool {
	return func(d int) bool {
		for _, x := range xs {
			if d == x {
				return true
			}
		}
		return false
	}
}

func evenUntil(space int)  func(*character) {
	return until(space, isEven)
}

func missTurn(n int)  func(*character) {
	return forTurns(n, func(int) bool { return false })
}

func moveBackwards(turns int)  func(*character) {
	return func(ch *character) {
		logf("backwards %d turns", turns)
		ch.backwards = true
		t := ch.turn + turns
		ch.checkMove = func(int) bool {
			if ch.turn > t {
				ch.checkMove = nil
				ch.backwards = false
			}
			return true
		}
	}
}

func needOneOfForTurns(turns int, need ...int) func(*character) {
	return untilSatisfied(turns, isOneOf(need...))
}

func needOneOf(need ...int) func(*character) {
	return untilSatisfied(1, isOneOf(need...))
}
