package main

import (
	"code.google.com/p/rog-go/exp/go/token"
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/sym"
	"fmt"
	"log"
)

type writeCmd struct {
	*context

	// lines holds all input lines.
	lines map[token.Position]*symLine

	// symPkgs holds packages that are mentioned in input
	// lines that request a change.
	symPkgs map[string]bool

	// globalReplace holds all the objects that
	// will be globally replaced and the new name
	// of the object's symbol.
	globalReplace map[*ast.Object]string

	// changed holds all the files that have been modified.
	changed map[*ast.File]bool
}

func init() {
	register("write", &writeCmd{}, nil)
}

func (wctxt *writeCmd) run(ctxt *context, args []string) error {
	wctxt.context = ctxt
	wctxt.lines = make(map[token.Position]*symLine)
	wctxt.symPkgs = make(map[string]bool)
	wctxt.globalReplace = make(map[*ast.Object]string)

	pkgs := args
	if err := wctxt.readSymbols(); err != nil {
		return fmt.Errorf("failed to read symbols: %v", err)
	}
	wctxt.addGlobals()
	wctxt.replace(pkgs)
	for name := range wctxt.ChangedFiles {
		wctxt.printf("%s\n", name)
	}
	if err := wctxt.WriteFiles(wctxt.ChangedFiles); err != nil {
		return err
	}
	return nil
}

// replace replaces all symbols in files as directed by
// the input lines.
func (wctxt *writeCmd) replace(pkgs []string) {
	visitor := func(info *sym.Info) bool {
		globSym, globRepl := wctxt.globalReplace[info.ReferObj]
		p := wctxt.position(info.Pos)
		p.Offset = 0
		line, lineRepl := wctxt.lines[p]
		if !lineRepl && !globRepl {
			return true
		}
		var newSym string
		if lineRepl {
			if newSym = line.symName(); newSym == info.ReferObj.Name {
				// There is a line for this symbol, but the name is
				// not changing, so ignore it.
				lineRepl = false
			}
		}
		if globRepl {
			// N.B. global symbols are not recorded in globalReplace
			// if they make no change.
			if lineRepl && globSym != newSym {
				log.Printf("gosym: %v: conflicting global/local change (%q vs %q)", p, globSym, newSym)
				return true
			}
			newSym = globSym
		}
		info.Ident.Name = newSym
		return true
	}
	for _, path := range pkgs {
		pkg := wctxt.Import(path)
		if pkg == nil {
			log.Printf("gosym: could not find package %q", path)
			continue
		}
		for _, f := range pkg.Files {
			// TODO when no global replacements, don't bother if file
			// isn't mentioned in input lines.
			wctxt.IterateSyms(f, visitor)
		}
	}
}

// addGlobals adds any symbols to wctxt.globalReplace that
// have a change requested by any input line.
func (wctxt *writeCmd) addGlobals() {
	// visitor adds a symbol to wctxt.globalReplace if necessary.
	visitor := func(info *sym.Info) bool {
		p := wctxt.position(info.Pos)
		p.Offset = 0
		line, ok := wctxt.lines[p]
		if !ok || !line.plus {
			return true
		}
		sym := line.symName()
		if sym != info.ReferObj.Name {
			// name being changed does not match object.
			log.Printf("gosym: %v: changing %q to %q; saw %q; p, ignoring", p, sym, line.newExpr, info.ReferObj.Name)
		}
		if old, ok := wctxt.globalReplace[info.ReferObj]; ok {
			if old != sym {
				log.Printf("gosym: %v: conflicting replacement for %s", p, line.expr)
				return true
			}
		}
		wctxt.globalReplace[info.ReferObj] = line.newExpr
		return true
	}

	// Search for all symbols that need replacing.
	for path := range wctxt.symPkgs {
		pkg := wctxt.Import(path)
		if pkg == nil {
			log.Printf("gosym: could not find package %q", path)
			continue
		}
		for _, f := range pkg.Files {
			// TODO don't bother if file isn't mentioned in input lines.
			wctxt.IterateSyms(f, visitor)
		}
	}
}

// readSymbols reads all the symbols from stdin.
func (wctxt *writeCmd) readSymbols() error {
	readLines(func(sl *symLine) error {
		if sl.long {
			return fmt.Errorf("line is not in short format")
		}
		if sl.newExpr == sl.symName() {
			// Ignore line if it doesn't request a change.
			return nil
		}
		if old, ok := wctxt.lines[sl.pos]; ok {
			log.Printf("%v: duplicate symbol location; original at %v", sl.pos, old.pos)
			return nil
		}
		wctxt.lines[sl.pos] = sl
		wctxt.symPkgs[wctxt.positionToImportPath(sl.pos)] = true
		return nil
	})
	return nil
}
