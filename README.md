# godef - find symbol information in Go source

Godef, given an expression or a location in a source file, prints the location of the definition of the symbol referred to.

## Known limitations

- It does not understand about "." imports
- It does not deal well with definitions in tests
