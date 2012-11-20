package main

import (
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/token"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type symLine struct {
	pos      token.Position // file address of identifier; addr.Offset is zero.
	referPos token.Position // file address of referred-to identifier.
	long     bool           // line is in long format.
	exprPkg  string         // package containing identifier (long format only)
	referPkg string         // package containing referred-to object (long format only)
	expr     string         // name of identifier. fully qualified.
	local    bool           // identifier is function-local (long format only)
	kind     ast.ObjKind    // kind of identifier (long format only)
	plus     bool           // line is, or refers to, definition of object. (long format only)
	exprType string         // type of expression (unparsed). (long format only)
	// valid in short form only.
	newExpr  string         // new name of identifier, unqualified.
}

// long format:
// filename.go:35:5: referfilename.go:2:4 pkg referPkg expr kind [type]
// short format:
// filename.go:35.5: expr newExpr

var linePat = regexp.MustCompile(`^` +
	`([^:]+):(\d+):(\d+):` + // 1,2,3: filename
	`(` +
	`\s+([^:]+):(\d+):(\d+)` + // 5,6,7: filename
	`\s+([^\s]+)` + // 8: exprPkg
	`\s+([^\s]+)` + // 9: referPkg
	`\s+([^\s]+)` + // 10: expr
	`\s+(local)?([^\s+]+)(\+)?` + // 11,12,13: local, kind, plus
	`(\s+([^\s].*))?` + // 15: exprType
	`|` +
	`\s+([^\s]+)` + // 16: expr
	`\s+([^\s]+)` + // 17: newExpr
	`)` +
	`$`)

func atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic("bad number")
	}
	return i
}

func parseSymLine(line string) (*symLine, error) {
	m := linePat.FindStringSubmatch(line)
	if m == nil {
		return nil, fmt.Errorf("invalid line")
	}
	var l symLine
	l.pos.Filename = m[1]
	l.pos.Line = atoi(m[2])
	l.pos.Column = atoi(m[3])
	if m[5] != "" {
		l.long = true
		l.referPos.Filename = m[5]
		l.referPos.Line = atoi(m[6])
		l.referPos.Column = atoi(m[7])
		l.exprPkg = m[8]
		l.referPkg = m[9]
		l.expr = m[10] // TODO check for invalid chars in expr
		l.local = m[11] == "local"
		var ok bool
		l.kind, ok = objKinds[m[12]]
		if !ok {
			return nil, fmt.Errorf("invalid kind %q", m[12])
		}
		l.plus = m[13] == "+"
		if m[15] != "" {
			l.exprType = m[15]
		}
	} else {
		l.expr = m[16]
		l.newExpr = m[17]
	}
	return &l, nil
}

func (l *symLine) String() string {
	if l.long {
		local := ""
		if l.local {
			local = "local"
		}
		def := ""
		if l.plus {
			def = "+"
		}
		exprType := ""
		if len(l.exprType) > 0 {
			exprType = " " + l.exprType
		}
		return fmt.Sprintf("%v: %v %s %s %s %s%s%s%s", l.pos, l.referPos, l.exprPkg, l.referPkg, l.expr, local, l.kind, def, exprType)
	}
	if l.newExpr == "" {
		panic("no new expr in short-form sym line")
	}
	return fmt.Sprintf("%v: %s %s", l.pos, l.expr, l.newExpr)
}

func (l *symLine) symName() string {
	if i := strings.LastIndex(l.expr, "."); i >= 0 {
		return l.expr[i+1:]
	}
	return l.expr
}
