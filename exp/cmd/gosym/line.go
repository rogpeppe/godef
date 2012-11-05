package main
import (
	"fmt"
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/token"
	"regexp"
	"strings"
	"strconv"
)

type symLine struct {
	pos      token.Position // file address of identifier; addr.Offset is zero.
	long bool			// line is in long format.
	exprPkg  string         // package containing identifier (long format only)
	referPkg string         // package containing referred-to object (long format only)
	expr     string         // name of identifier. fully qualified.
	local    bool           // identifier is function-local (long format only)
	kind     ast.ObjKind    // kind of identifier (long format only)
	plus     bool           // line is, or refers to, definition of object. (long format only)
	newExpr string		// new name of identifier, unqualified.
	exprType string         // type of expression (unparsed). (long format only)
}

// long format:
// filename.go:35:5: pkg referPkg expr kind [type]
// short format:
// filename.go:35.5: expr newExpr

var linePat = regexp.MustCompile(`^`+
	`([^:]+):(\d+):(\d+):`+			// 1,2,3: filename
	`(`+
	`\s+([^\s]+)`+				// 5: exprPkg
	`\s+([^\s]+)`+				// 6: referPkg
	`\s+([^\s]+)`+				// 7: expr
	`\s+(local)?([^\s+]+)(\+)?`+	// 8, 9, 10: local, kind, plus
	`(\s+([^\s].*))?`+			// 12: exprType
	`|`+
	`\s+([^\s]+)`+				// 13: expr
	`\s+([^\s]+)`+				// 14: newExpr
	`)`+
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
		return nil, fmt.Errorf("invalid line %q", line)
	}
	var l symLine
	l.pos.Filename = m[1]
	l.pos.Line = atoi(m[2])
	l.pos.Column = atoi(m[3])
	if m[5] != "" {
		l.long = true
		l.exprPkg = m[5]
		l.referPkg = m[6]
		l.expr = m[7] // TODO check for invalid chars in expr
		l.local = m[8] == "local"
		var ok bool
		l.kind, ok = objKinds[m[9]]
		if !ok {
			return nil, fmt.Errorf("invalid kind %q", m[9])
		}
		l.plus = m[10] == "+"
		if m[12] != "" {
			l.exprType = m[12]
		}
	} else {
		l.expr = m[13]
		l.newExpr = m[14]
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
		return fmt.Sprintf("%v: %s %s %s %s%s%s%s", l.pos, l.exprPkg, l.referPkg, l.expr, local, l.kind, def, exprType)
	}
	return fmt.Sprintf("%v: %s %s", l.pos, l.expr, l.newExpr)
}

func (l *symLine) symName() string {
	if i := strings.LastIndex(l.expr, "."); i >= 0 {
		return l.expr[i+1:]
	}
	return l.expr
}
