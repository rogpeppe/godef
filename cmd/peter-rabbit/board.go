package main

func newCharacter(paths map[string][]square) *character {
	ch := &character{
		paths: paths,
		roll: roll,
	}
	ch.pos = findPos(ch, "start", 1)
	return ch
}

var peterPaths = map[string][]square{
	"start": []square{
		5: {
			land: jumpTo("start", 1),
		},
		7: {
			land: jumpTo("under gate", 1),
		},
		18: {
			halt: true,
			land: oddUntil(34),
		},
		35: {
			land: jumpTo("short cut from 35", 1),
		},
		41: {
			land: missTurn(1),
		},
		50: {
			halt: true,
		},
		51: {
			land: needOneOf(3, 5),		// ??
		},
		64: {
			land: missTurn(1),
		},
		71: {
			land: jumpTo("start", 53),
		},
		74: {
			land: moveBackwards(2),
		},
		80: {
			land: missTurn(2),
		},
		83: {
			halt: true,
			land: needOneOf(2,4,6),
		},
		89: {
			land: jumpTo("start", 80),
		},
		111: {
			land: jumpTo("start", 116),
		},
		122: {},
	},
	"under gate": []square{
		7: {
			join: joinPos("start", 17),
		},
	},
	"short cut from 35": []square{
		6: {
			join: joinPos("start", 49),
		},
	},
}

var squirrelPaths = map[string][]square{
	"start": []square{
		8: {
			land: jumpTo("start", 1),
		},
		12: {
			land: missTurn(1),
		},
		17: {
			halt: true,
			land: evenOddAlternate(25),
		},
		23: {
			land: jumpTo("start", 17),
		},
		25: {
			halt: true,
			land: combine(
				jumpTo("lake", 1), 
				needOneOf(6)),
		},
		31: {
			land: jumpTo("start", 25),
		},
		37: {
			land: combine(
				jumpTo("trees", 1),
				oddUntil(15)),
		},
		47: {
			land: missTurn(2),
		},
		54: {
			land: jumpTo("start", 52),
		},
		59: {
			land: extraRolls(1),
		},
		69: {
			land: jumpTo("start", 81),
		},
		77: {
			land: until(96, isEven),
		},
		96: {
			land: jumpTo("start", 98),
		},
		99: {
			halt: true,
		},
		107: {
			land: jumpTo("start", 113),
		},
		122: {},
	},
	"lake": []square{
		7: {
			join: joinPos("start", 26),
		},
	},
	"trees": []square{
		15: {
			join: joinPos("start", 45),
		},
	},
}

var jeremyPaths = map[string][]square{
	"start": []square{
		7: {
			land: missTurn(2),
		},
		8: {
			land: jumpTo("start", 11),
		},
		12: {
			land: jumpTo("start", 7),
		},
		22: {
			land: jumpTo("start", 28),
		},
		28: {
			land: jumpTo("start", 54),
		},
		33: {
			halt: true,
			land: needOneOfForTurns(2, 1),
		},
		41: {
			land: jumpTo("side stream", 1),
		},
		52: {
			land: jumpTo("start", 40),
		},
		56: {
			halt: true,
		},
		57: {
			land: jumpTo("start", 62),
		},
		68: {
			land: jumpTo("start", 54),
		},
		79: {
			land: missTurn(2),
		},
		85: {
			land: jumpTo("start", 88),
		},
		88: {
			land: evenUntil(103),
		},
		112: {
			land: moveBackwards(2),
		},
		122: {},
	},
	"side stream": []square{
		8: {
			join: joinPos("start", 51),
		},
	},
}

var jemimaPaths =  map[string][]square{
	"start": {
		6: {
			land: jumpTo("start", 1),
		},
		12: {
			land: jumpTo("start", 20),
		},
		26: {
			land: jumpTo("long way round", 1),
		},
		34: {
			land: missTurn(2),
		},
		42: {
			land: jumpTo("cart road", 1),
		},
		50: {
			land: jumpTo("start", 35),
		},
		55: {
			land: needOneOf(6),
		},
		66: {
			land: jumpTo("start", 77),
		},
		78: {
			halt: true,
			land: until(90, isOneOf(2, 3)),
		},
		90: {
			land: missTurn(2),
		},
		110: {
			halt: true,
		},
		113: {
			land: jumpTo("start", 123),
		},
		122: {},
	},
	"long way round": {
		22: {
			join: joinPos("start", 34),
		},
	},
	"cart road": {
		13: {
			join: joinPos("start", 48),
		},
	},

}