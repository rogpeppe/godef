package main

import (
	"code.google.com/p/rog-go/exp/go/ast"
	"code.google.com/p/rog-go/exp/go/sym"
	"code.google.com/p/rog-go/exp/go/token"
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

var writeAbout = `
gosym write [pkg...]

The gosym command reads lines in short format (see the
"short" subcommand) from its standard input
that represent changes to make, and changes any of the
named packages accordingly - that is, the identifier
at each line's file-position (and all uses of it) is changed to the new-name
field.

If no packages are named, "." is used. No files outside the named packages
will be changed. The names of any changed files will
be printed.

As with gofix, writes are destructive - make sure your
source files are backed up before using this command.
`[1:]

func init() {
	register("write", &writeCmd{}, nil, writeAbout)
}

func (c *writeCmd) run(ctxt *context, args []string) error {
	c.context = ctxt
	c.lines = make(map[token.Position]*symLine)
	c.symPkgs = make(map[string]bool)
	c.globalReplace = make(map[*ast.Object]string)

	pkgs := args
	if err := c.readSymbols(); err != nil {
		return fmt.Errorf("failed to read symbols: %v", err)
	}
	c.addGlobals()
	c.replace(pkgs)
	for name := range c.ChangedFiles {
		c.printf("%s\n", name)
	}
	if err := c.WriteFiles(c.ChangedFiles); err != nil {
		return err
	}
	return nil
}

// readSymbols records all the symbols from stdin.
func (c *writeCmd) readSymbols() error {
	readLines(func(sl *symLine) error {
		if sl.long {
			return fmt.Errorf("line is not in short format")
		}
		if sl.newExpr == sl.symName() {
			// Ignore line if it doesn't request a change.
			return nil
		}
		if old, ok := c.lines[sl.pos]; ok {
			log.Printf("%v: duplicate symbol location; original at %v", sl.pos, old.pos)
			return nil
		}
		c.lines[sl.pos] = sl
		c.symPkgs[c.positionToImportPath(sl.pos)] = true
		return nil
	})
	return nil
}

// addGlobals adds any symbols to wctxt.globalReplace that
// have a change requested by any input line.
func (c *writeCmd) addGlobals() {
	// visitor adds a symbol to wctxt.globalReplace if necessary.
	visitor := func(info *sym.Info) bool {
		p := c.position(info.Pos)
		p.Offset = 0
		line, ok := c.lines[p]
		if !ok {
			return true
		}
		sym := line.symName()
		if sym != info.ReferObj.Name {
			// name being changed does not match object.
			log.Printf("gosym: %v: changing %q to %q; saw %q; p, ignoring", p, sym, line.newExpr, info.ReferObj.Name)
		}
		if old, ok := c.globalReplace[info.ReferObj]; ok {
			if old != line.newExpr {
				log.Printf("gosym: %v: conflicting replacement for %s", p, line.expr)
				return true
			}
		}
		c.globalReplace[info.ReferObj] = line.newExpr
		return true
	}

	// Search for all symbols that need replacing.
	for path := range c.symPkgs {
		pkg := c.Import(path)
		if pkg == nil {
			log.Printf("gosym: could not find package %q", path)
			continue
		}
		for _, f := range pkg.Files {
			// TODO don't bother if file isn't mentioned in input lines.
			c.IterateSyms(f, visitor)
		}
	}
}

// replace replaces all symbols in files as directed by
// the input lines.
func (c *writeCmd) replace(pkgs []string) {
	if len(pkgs) == 0 {
		pkgs = []string{"."}
	}
	visitor := func(info *sym.Info) bool {
		globSym, globRepl := c.globalReplace[info.ReferObj]
		p := c.position(info.Pos)
		p.Offset = 0
		line, lineRepl := c.lines[p]
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
		pkg := c.Import(path)
		if pkg == nil {
			log.Printf("gosym: could not find package %q", path)
			continue
		}
		for _, f := range pkg.Files {
			// TODO when no global replacements, don't bother if file
			// isn't mentioned in input lines.
			c.IterateSyms(f, visitor)
		}
	}
}
