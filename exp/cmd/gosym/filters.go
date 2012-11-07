package main

import (
	"bufio"
	"code.google.com/p/rog-go/exp/go/token"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"unicode"
)

func readLines(f func(sl *symLine) error) error {
	r := bufio.NewReader(os.Stdin)
	for {
		line, isPrefix, err := r.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read error: %v", err)
		}
		if isPrefix {
			return fmt.Errorf("line too long")
		}
		sl, err := parseSymLine(string(line))
		if err != nil {
			return fmt.Errorf("cannot parse line %q: %v", line, err)
		}
		if err := f(sl); err != nil {
			return err
		}
	}
	return nil
}

func runSimpleFilter(ctxt *context, f func(string) string) error {
	return readLines(func(sl *symLine) error {
		if sl.long {
			sl.newExpr = sl.symName()
		}
		sl.newExpr = f(sl.newExpr)
		sl.long = false
		ctxt.printf("%s\n", sl)
		return nil
	})
}

type shortCmd struct{}

func init() {
	register("short", &shortCmd{}, nil, `
gosym short

The short command reads lines from standard input
in short or long format (see the list command) and
prints them in short format:
	file-position name new-name
The file-position field holds the location of the identifier.
The name field holds the name of the identifier (in X.Y format if
it is defined as a member of another type X).
The new-name field holds the desired new name for the identifier.
`[1:])
}

func (c *shortCmd) run(ctxt *context, args []string) error {
	return readLines(func(sl *symLine) error {
		if sl.long {
			sl.newExpr = sl.symName()
		}
		sl.long = false
		ctxt.printf("%s\n", sl)
		return nil
	})
}

type exportCmd struct{}

func init() {
	register("export", &exportCmd{}, nil, `
gosym export

The export command reads lines in short or long
format from its standard input and capitalises the first letter
of all symbols (thus making them available to external
packages)

Note that this may cause clashes with other symbols
that have already been defined with the new capitalisation.
`[1:])
}

func (c *exportCmd) run(ctxt *context, args []string) error {
	return runSimpleFilter(ctxt, toExported)
}

func toExported(s string) string {
	first := true
	return strings.Map(func(r rune) rune {
		if first {
			first = false
			return unicode.ToUpper(r)
		}
		return r
	}, s)
}

type unexportCmd struct{}

func init() {
	register("unexport", &unexportCmd{}, nil, `
gosym unexport

The unexport command reads lines in short or long
format from its standard input and uncapitalises the first letter
of all symbols (thus making them unavailable to external
packages).

Note that this may cause clashes with other symbols
that have already been defined with the new capitalisation.
`[1:])
}

func (c *unexportCmd) run(ctxt *context, args []string) error {
	return runSimpleFilter(ctxt, toUnexported)
}

func toUnexported(s string) string {
	first := true
	return strings.Map(func(r rune) rune {
		if first {
			first = false
			return unicode.ToLower(r)
		}
		return r
	}, s)
}

type renameCmd struct{}

func init() {
	register("rename", &renameCmd{}, nil, `
gosym rename [old new]...

The rename command renames any symbol with the
given old name to the given new name. The
qualifier symbol's qualifier is ignored.

Note that this may cause clashes with other symbols
that have already been defined with the new name.
`[1:])

}

func (c *renameCmd) run(ctxt *context, args []string) error {
	if len(args)%2 != 0 {
		return fmt.Errorf("rename requires even number of arguments")
	}
	from := make(map[string]string)
	for i := 0; i < len(args); i += 2 {
		from[args[i]] = args[i+1]
	}
	return runSimpleFilter(ctxt, func(s string) string {
		if to, ok := from[s]; ok {
			return to
		}
		return s
	})
}

func readUses(pkgArgs []string) (defs map[token.Position]*symLine, uses map[token.Position]*symLine, err error) {
	if len(pkgArgs) == 0 {
		return nil, nil, fmt.Errorf("at least one package argument required")
	}
	pkgs := make(map[string]bool)
	for _, a := range pkgArgs {
		pkgs[a] = true
	}
	defs = make(map[token.Position]*symLine)
	uses = make(map[token.Position]*symLine)
	err = readLines(func(sl *symLine) error {
		if !sl.long {
			return fmt.Errorf("input must be in long format")
		}
		if pkgs[sl.exprPkg] {
			if sl.plus {
				defs[sl.pos] = sl
			}
		} else if pkgs[sl.referPkg] {
			uses[sl.referPos] = sl
		}
		return nil
	})
	return
}

type usedCmd struct{}

func init() {
	// used reads lines in long format; prints any definitions (in long format)
	// found in pkgs that are used by any other packages.
	register("used", &usedCmd{}, nil, `
gosym used pkg...

The used command reads lines in long format from the standard input and
prints (in long format) any definitions found in the named packages that
have references to them from any other package.
`[1:])
}

func (c *usedCmd) run(ctxt *context, args []string) error {
	defs, uses, err := readUses(args)
	if err != nil {
		return err
	}
	for use, usl := range uses {
		if sl := defs[use]; sl != nil {
			ctxt.printf("%s\n", sl)
		} else {
			log.Printf("definition for %v not found; used at %v", use, usl.pos)
		}
	}
	return nil
}

type unusedCmd struct{}

func init() {
	// unused reads lines in long format; prints any definitions (in long format)
	// found in pkgs that are used by any other packages.
	register("unused", &unusedCmd{}, nil, `
gosym unused pkg...

The unused command reads lines in long format from the standard input and
prints (in long format) any definitions found in the named packages that
have no references to them from any other package.
`[1:])
}

func (c *unusedCmd) run(ctxt *context, args []string) error {
	defs, uses, err := readUses(args)
	if err != nil {
		return err
	}
	for def, sl := range defs {
		if uses[def] == nil {
			ctxt.printf("%s\n", sl)
		}
	}
	return nil
}
