# godef - find symbol information in Go source

Godef, given an expression or a location in a source file, prints the
location of the definition of the symbol referred to.

## Installation

Run `go get github.com/rogpeppe/godef`

## Known limitations

- it does not understand about "." imports
- it does not deal well with definitions in tests.
