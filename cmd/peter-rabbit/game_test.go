package main
import (
	"errors"
	"log"
	"testing"
	"math/rand"
)

var pathTests = []struct {
	about string
	start int
	finish int
	paths map[string][]square
	turns int
	dice []int
} {{
	about: "all ones",
	paths: map[string][]square{
		"start": make([]square, 10),
	},
	dice: []int{1,2,3,4,5,6,1,1,1,1,1,1,1,1,1},
	turns: 14,
}, {
	about: "need exact to end",
	paths: map[string][]square{
		"start": make([]square, 10),
	},
	dice: []int{6, 5, 3, 4, 5, 6, 5, 1},
	turns: 6,
},{
	about: "jump to",
	paths: map[string][]square{
		"start": []square{
			4: {
				land: jumpTo("start", 2),
			},
			6: {},
		},
	},
	dice: []int{6, 3, 2, 2, 5},
	turns: 4,
}, {
	about: "jump to with join",
	paths: map[string][]square{
		"start": []square{
			4: {
				land: jumpTo("other", 1),
			},
			6: {},
		},
		"other": []square{
			1: {
				land: jumpTo("other", 3),
			},
			5: {
				join: joinPos("start", 5),
			},
		},
	},
	dice: []int{6, 1, 2, 3, 1},
	turns: 4,
}, {
	about: "ignore land on 6",
	paths: map[string][]square{
		"start": []square{
			7: {
				land: jumpTo("start", 1),
			},
			8: {},
		},
	},
	dice: []int{6, 6, 2},
	turns: 1,
}, {
	about: "odd until",
	paths: map[string][]square{
		"start": []square{
			4: {
				land: oddUntil(10),
			},
			13: {},
		},
	},
	dice: []int{6, 1, 2, 2, 4, 6, 6, 4, 1, 3, 2, 3, 3},
	turns: 10,
}, {
	about: "odd until with sixes",
	paths: map[string][]square{
		"start": []square{
			4: {
				land: oddUntil(10),
			},
			13: {},
		},
	},
	dice: []int{6, 1, 2, 2, 4, 6, 6, 4, 1, 3, 2, 3, 3},
	turns: 10,
}, {
	about: "miss two turns",
	paths: map[string][]square{
		"start": []square{
			2: {
				land: missTurn(2),
			},
			4: {},
		},
	},
	dice: []int{6, 1, 6, 6, 6, 3, 5, 3},
	turns: 4,
}, {
	about: "extra roll",
	paths: map[string][]square{
		"start": []square{
			2: {
				land: extraRolls(1),
			},
			10: {},
		},
	},
	dice: []int{6,1,3,4,2},
	turns: 3,
}, {
	about: "need one of",
	paths: map[string][]square{
		"start": []square{
			2: {
				land: needOneOf(1,2,5),
			},
			4: {},
		},
	},
	dice: []int{6, 1, 6, 6, 6, 3, 4, 1, 2},
	turns: 5,
}, {
	about: "combine",
	paths: map[string][]square{
		"start": []square{
			2: {
				land: combine(
					jumpTo("other", 1),
					needOneOf(6)),
			},
			4: {},
		},
		"other": []square{
			7: {
				join: joinPos("start", 3),
			},
		},
	},
	dice: []int{6, 1, 1,2,3,4,5,6,2},
	turns: 7,
},{
	about: "even-odd alternate",
	paths: map[string][]square{
		"start": []square{
			2: {
				land: evenOddAlternate(10),
			},
			12: {},
		},
	},
	dice: []int{
		6, 1,
		1, 3, 5,
		2,
		2, 4, 6,
		3,
		3, 5, 1,
		4,
		2,
	},
	turns: 13,
}, {
	about: "need one of for turns",
	paths: map[string][]square{
		"start": []square{
			2: {
				land: needOneOfForTurns(2, 2, 3),
			},
			12: {},
		},
	},
	dice: []int{6,1,1,5,6,6,1,4,2,4,3,6},
	turns: 9,
}, {
	about: "halt",
	paths: map[string][]square{
		"start": []square{
			2: {
				halt: true,
			},
			4: {},
		},
	},
	dice: []int{6, 5, 3},
	turns: 2,
}, {
	about: "halt with need one of",
	paths: map[string][]square{
		"start": []square{
			2: {
				halt: true,
				land: needOneOf(1),
			},
			4: {},
		},
	},
	dice: []int{6, 5, 4, 6, 6, 6, 3, 1, 2},
	turns: 5,
}, {
	about: "backwards",
	paths: map[string][]square{
		"start": []square{
			20: {
				land: moveBackwards(2),
			},
			21: {},
		},
	},
	dice: []int{6, 6, 6, 6, 1, 4, 5, 6, 3, 2, 3, 3, 4},
	turns: 8,
},
//	about: "jeremy",
//	paths: jeremyPaths,
//	turns: ??,
//	dice: []int{
//		6, 5,
//		2, 		// wait two turns
//		1, 1,
//		1,		// jump to 11
//		1,		// back to 7, wait two turns
//		5, 5,
//		4, 5, 5, 1,	// go to 28, jump to 54
//		5,		// halt at 56
//		1,		// jump to 62
//		5, 1,		// jump to 54
//		6, 6, 6, 6, 5,
//		2,
//	},
{
	about: "peter, 1-38",
	paths: peterPaths,
	finish: 38,
	turns: 12,
	dice: []int{
		6, 4,		// 5: go back to start
		5,
		1,		// 7: short cut to 17
		4, 5,		// 18: halt, odd until 34
		4,		// ignore
		5,		// 23
		4,		// ignore
		5,		// 28
		6,2,		// ignore
		5,		// 33
		5,
	},
}, {
	about: "peter 32-53",
	paths: peterPaths,
	start: 32,
	finish: 54,
	turns: 9,
	dice: []int{
		6,
		3,		// 35: short cut to 49
		3,
		5,		// halt at 50
		1, 		// 3 or 5 to get on again
		1,2,4,6,4,
		3,
	},
}, {
	about: "peter 62-66",
	paths: peterPaths,
	start: 62,
	finish: 66,
	turns: 3,
	dice: []int{
		6,
		2,		// 64: wait one turn
		6,4,
		2,
	},
},  {
	about: "peter 52-72",
	paths: peterPaths,
	start: 52,
	finish: 72,
	turns: 8,
	dice: []int{
		6,
		5,
		5,
		5,
		4,	// 71: go back to 53
		5,
		5,
		5,
		4,
	},
}, {
	about: "peter 67-77",
	paths: peterPaths,
	start: 67,
	finish: 77,
	turns: 6,
	dice: []int{
		6,
		5,
		2,	// 74: count backwards two turns
		2,
		4,	// 68: forwards again
		5,
		4,
	},
},
}

var tooManyRolls = errors.New("too many rolls")

func roller(dice []int) (roll func() int, remain func() []int) {
	return func() int {
		if len(dice) == 0 {
			panic(tooManyRolls)
		}
		d := dice[0]
		dice = dice[1:]
		return d
	}, func() []int {
		return dice
	}
}

func TestPaths(t *testing.T) {
	defer func() {
		if e := recover(); e != nil {
			if e == tooManyRolls {
				t.Errorf("too many rolls")
			} else {
				panic(e)
			}
		}
	}()
	for i, test := range pathTests {
		log.Printf("----- test %d: %s", i, test.about)
		if test.finish != 0 {
			test.paths["start"] = test.paths["start"][0:test.finish]
		}
		t.Logf(" test %d: %s", i, test.about)
		roll, remain := roller(test.dice)
		ch := newCharacter(test.paths)
		ch.roll = roll
		if test.start != 0 {
			ch.pos.i = test.start
		}
		n := ch.play()
		if n != test.turns {
			t.Errorf("expect %d turns; got %d", test.turns, n)
		}
		if r := remain(); len(r) > 0 {
			t.Errorf("%d remaining dice", len(r))
		}
	}
}

func BenchmarkOne(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rand.Seed(0)
		ch := newCharacter(peterPaths)
		ch.play()
	}
}
