// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*

Godef prints the source location of definitions in Go programs.

Usage:

	godef [-o offset] [-b] [-i] file [expr]

File specifies the source file in which to evaluate expr.
Expr must be an identifier, or a Go expression
terminated with a field selector.

If expr is not given, then offset specifies a location
within file, which should be within, or adjacent to
an identifier or field selector. By default the location
is specified in unicode characters; the -b flag
causes it to be interpreted in bytes.

If the -i flag is specified, the source is read
from standard input, although file must still
be specified so that other files in the same source
package may be found.

Example:

	$ cd $GOROOT
	$ godef src/pkg/xml/read.go 'NewParser().Skip'
	src/pkg/xml/read.go:384:18
	$

The acme directory in the godef source holds
some files to enable godef to be used inside the acme editor.

*/

package documentation
