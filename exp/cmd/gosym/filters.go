package main
import (
	"bufio"
	"os"
	"io"
	"fmt"
	"strings"
	"log"
	"unicode"
	"code.google.com/p/rog-go/exp/go/token"
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
		return nil
	})
}

type shortCmd struct {}

func init() {
	register("short", &shortCmd{}, nil)
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

type longCmd struct {}

func init() {
	register("long", &longCmd{}, nil)
}

func (c *longCmd) run(ctxt *context, args []string) error {
	return readLines(func(sl *symLine) error {
		if !sl.long {
			return fmt.Errorf("not in long format")
		}
		ctxt.printf("%s\n", sl)
		return nil
	})
}


type exportCmd struct {}

func init() {
	register("export", &exportCmd{}, nil)
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

type unexportCmd struct {}

func init() {
	register("unexport", &unexportCmd{}, nil)
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

type renameCmd struct {}

func init() {
	register("rename", &renameCmd{}, nil)
}

func (c *renameCmd) run(ctxt *context, args []string) error {
	if len(args) % 2 != 0 {
		return fmt.Errorf("rename requires even number of arguments")
	}
	from := make(map[string]string)
	for i := 0; i < len(args); i += 2 {
		from[args[i]] = args[i+1]
	}
	runSimpleFilter(ctxt, func(s string) string {
		if to, ok := from[s]; ok {
			return to
		}
		return s
	})
	return nil
}


func readUses(pkgArgs []string) (defs map[token.Position] *symLine, uses map[token.Position] *symLine, err error) {
	if len(pkgArgs) == 0 {
		return nil, nil, fmt.Errorf("at least one package argument required")
	}
	pkgs := make(map[string]bool)
	for _, a := range pkgArgs {
		pkgs[a] = true
	}
	defs = make(map[token.Position] *symLine)
	uses = make(map[token.Position] *symLine)
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

type usedCmd struct {}

func init() {
	// used reads lines in long format; prints any definitions (in long format)
	// found in pkgs that are used by any other packages.
	register("used", &usedCmd{}, nil)
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

type unusedCmd struct {}

func init() {
	// unused reads lines in long format; prints any definitions (in long format)
	// found in pkgs that are used by any other packages.
	register("unused", &unusedCmd{}, nil)
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