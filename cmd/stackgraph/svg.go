package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"
)

func (n *Node) ArgCounts() string {
	counts := make([]string, len(n.ArgCount))
	for i, m := range n.ArgCount {
		if len(m) == 1 {
			for val, _ := range m {
				counts[i] = fmt.Sprintf("=%#x", val)
			}
		} else {
			counts[i] = fmt.Sprint(len(m))
		}
	}
	return fmt.Sprintf("%s", counts)
}

// Most of this code was purloined from go tool pprof.

type Summary struct {
	Title      string
	Edges      map[Arc]*Edge
	Nodes      map[string]*Node
	TotalCalls int
	TotalEdges int
}

func writeSVG(w io.Writer, summary *Summary) error {
	dotcmd := exec.Command("dot", "-Tsvg")
	var output bytes.Buffer
	dotcmd.Stdout = &output
	dotcmd.Stderr = os.Stderr
	pr, pw := io.Pipe()
	dotcmd.Stdin = pr
	if err := dotcmd.Start(); err != nil {
		log.Fatalf("cannot exec dot: %v", err)
	}
	if err := dotTemplate.Execute(pw, summary); err != nil {
		log.Fatal(err)
	}
	pw.Close()
	if err := dotcmd.Wait(); err != nil {
		log.Fatalf("dot failed: %v", err)
	}
	if _, err := os.Stdout.Write(rewriteSVG(output.Bytes())); err != nil {
		return err
	}
	return nil
}

func fontSize(x, y int) float64 {
	if y == 0 {
		return 0
	}
	return 8 + (50 * math.Sqrt(float64(x)/float64(y)))
}

func lineAttrs(x, y int) string {
	var frac float64
	if y == 0 {
		frac = 0
	} else {
		frac = 3 * float64(x) / float64(y)
	}
	if frac > 1 {
		// SVG output treats line widths < 1 poorly.
		frac = 1
	}
	w := frac * 2
	if w < 1 {
		w = 1
	}
	// Dot sometimes segfaults if given edge weights that are too large, so
	// we cap the weights at a large value
	edgeWeight := math.Pow(float64(x), 0.7)
	if edgeWeight > 100000 {
		edgeWeight = 100000
	}
	edgeWeight = math.Floor(edgeWeight)
	return fmt.Sprintf(`weight=%g, style="setlinewidth(%f)"`, edgeWeight, w)
}

// See dot(1).
var dotTemplate = template.Must(template.New("").Funcs(template.FuncMap{
	"fontSize":  fontSize,
	"lineAttrs": lineAttrs,
}).Parse(`
	digraph {{.Title|printf "%q"}} {
	size="8,11"
	node [width=0.375,height=0.25];
	{{range $n := .Nodes}}
	N{{$n.Id}} [
		label={{printf "%s [%d]\n%s" $n.Func $n.Count $n.ArgCounts | printf "%q"}},
		shape=box,
		fontsize={{fontSize $n.Count $.TotalCalls}},
	];
	{{end}}
	{{range $arc, $e := .Edges}}
	N{{$arc.Node0.Id}} -> N{{$arc.Node1.Id}} [label={{$e.Count}}, {{lineAttrs $e.Count $.TotalEdges}}];
	{{end}}
}
`))

func rewriteSVG(data []byte) []byte {
	//  Dot's SVG output is
	//
	//     <svg width="___" height="___"
	//      viewBox="___" xmlns=...>
	//     <g id="graph0" transform="...">
	//     ...
	//     </g>
	//     </svg>
	//
	//  Change it to
	//
	//     <svg width="100%" height="100%"
	//      xmlns=...>
	//     $svg_javascript
	//     <g id="viewport" transform="translate(0,0)">
	//     <g id="graph0" transform="...">
	//     ...
	//     </g>
	//     </g>
	//     </svg>

	//  Fix width, height; drop viewBox.
	data = regexpReplace(data,
		`(?s)<svg width="[^"]+" height="[^"]+"(.*?)viewBox="[^"]+"`,
		`<svg width="100%" height="100%"$1`)

	// Insert script, viewport <g> above first <g>
	viewport := `<g id="viewport" transform="translate(0,0)">
`
	data = regexpReplace(data, `<g id="graph\d"(.*?)`, svgJavascript+viewport+"$0")

	// Insert final </g> above </svg>.
	data = regexpReplace(data, `(.*)(</svg>)`, `$1</g>$2`)
	data = regexpReplace(data, `<g id="graph\d"(.*?)`, `<g id="viewport"$1`)
	return data
}

func regexpReplace(data []byte, re string, replacement string) []byte {
	rec := regexp.MustCompile(re)
	return rec.ReplaceAll(data, []byte(replacement))
}

func init() {
	if strings.Contains(svgJavascript, "$") {
		panic("javascript contains $ - can't be used as regexp substitute")
	}
}

const svgJavascript = `
<script type="text/ecmascript"><![CDATA[
// SVGPan
// http://www.cyberz.org/blog/2009/12/08/svgpan-a-javascript-svg-panzoomdrag-library/
// Local modification: if(true || ...) below to force panning, never moving.
// Local modification: add clamping to fix bug in handleMouseWheel.

/**
 *  SVGPan library 1.2
 * ====================
 *
 * Given an unique existing element with id "viewport", including the
 * the library into any SVG adds the following capabilities:
 *
 *  - Mouse panning
 *  - Mouse zooming (using the wheel)
 *  - Object dargging
 *
 * Known issues:
 *
 *  - Zooming (while panning) on Safari has still some issues
 *
 * Releases:
 *
 * 1.2, Sat Mar 20 08:42:50 GMT 2010, Zeng Xiaohui
 *	Fixed a bug with browser mouse handler interaction
 *
 * 1.1, Wed Feb  3 17:39:33 GMT 2010, Zeng Xiaohui
 *	Updated the zoom code to support the mouse wheel on Safari/Chrome
 *
 * 1.0, Andrea Leofreddi
 *	First release
 *
 * This code is licensed under the following BSD license:
 *
 * Copyright 2009-2010 Andrea Leofreddi <a.leofreddi@itcharm.com>. All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without modification, are
 * permitted provided that the following conditions are met:
 *
 *    1. Redistributions of source code must retain the above copyright notice, this list of
 *       conditions and the following disclaimer.
 *
 *    2. Redistributions in binary form must reproduce the above copyright notice, this list
 *       of conditions and the following disclaimer in the documentation and/or other materials
 *       provided with the distribution.
 *
 * THIS SOFTWARE IS PROVIDED BY Andrea Leofreddi ''AS IS'' AND ANY EXPRESS OR IMPLIED
 * WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND
 * FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL Andrea Leofreddi OR
 * CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
 * SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
 * ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 * NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF
 * ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 *
 * The views and conclusions contained in the software and documentation are those of the
 * authors and should not be interpreted as representing official policies, either expressed
 * or implied, of Andrea Leofreddi.
 */

var root = document.documentElement;

var state = 'none', stateTarget, stateOrigin, stateTf;

setupHandlers(root);

/**
 * Register handlers
 */
function setupHandlers(root){
	setAttributes(root, {
		"onmouseup" : "add(evt)",
		"onmousedown" : "handleMouseDown(evt)",
		"onmousemove" : "handleMouseMove(evt)",
		"onmouseup" : "handleMouseUp(evt)",
		//"onmouseout" : "handleMouseUp(evt)", // Decomment this to stop the pan functionality when dragging out of the SVG element
	});

	if(navigator.userAgent.toLowerCase().indexOf('webkit') >= 0)
		window.addEventListener('mousewheel', handleMouseWheel, false); // Chrome/Safari
	else
		window.addEventListener('DOMMouseScroll', handleMouseWheel, false); // Others

	var g = svgDoc.getElementById("svg");
	g.width = "100%";
	g.height = "100%";
}

/**
 * Instance an SVGPoint object with given event coordinates.
 */
function getEventPoint(evt) {
	var p = root.createSVGPoint();

	p.x = evt.clientX;
	p.y = evt.clientY;

	return p;
}

/**
 * Sets the current transform matrix of an element.
 */
function setCTM(element, matrix) {
	var s = "matrix(" + matrix.a + "," + matrix.b + "," + matrix.c + "," + matrix.d + "," + matrix.e + "," + matrix.f + ")";

	element.setAttribute("transform", s);
}

/**
 * Dumps a matrix to a string (useful for debug).
 */
function dumpMatrix(matrix) {
	var s = "[ " + matrix.a + ", " + matrix.c + ", " + matrix.e + "\n  " + matrix.b + ", " + matrix.d + ", " + matrix.f + "\n  0, 0, 1 ]";

	return s;
}

/**
 * Sets attributes of an element.
 */
function setAttributes(element, attributes){
	for (i in attributes)
		element.setAttributeNS(null, i, attributes[i]);
}

/**
 * Handle mouse move event.
 */
function handleMouseWheel(evt) {
	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	var delta;

	if(evt.wheelDelta)
		delta = evt.wheelDelta / 3600; // Chrome/Safari
	else
		delta = evt.detail / -90; // Mozilla

	var z = 1 + delta; // Zoom factor: 0.9/1.1

	// Clamp to reasonable values.
	// The 0.1 check is important because
	// a very large scroll can turn into a
	// negative z, which rotates the image 180 degrees.
	if(z < 0.1)
		z = 0.1;
	if(z > 10.0)
		z = 10.0;

	var g = svgDoc.getElementById("viewport");

	var p = getEventPoint(evt);

	p = p.matrixTransform(g.getCTM().inverse());

	// Compute new scale matrix in current mouse position
	var k = root.createSVGMatrix().translate(p.x, p.y).scale(z).translate(-p.x, -p.y);

        setCTM(g, g.getCTM().multiply(k));

	stateTf = stateTf.multiply(k.inverse());
}

/**
 * Handle mouse move event.
 */
function handleMouseMove(evt) {
	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	var g = svgDoc.getElementById("viewport");

	if(state == 'pan') {
		// Pan mode
		var p = getEventPoint(evt).matrixTransform(stateTf);

		setCTM(g, stateTf.inverse().translate(p.x - stateOrigin.x, p.y - stateOrigin.y));
	} else if(state == 'move') {
		// Move mode
		var p = getEventPoint(evt).matrixTransform(g.getCTM().inverse());

		setCTM(stateTarget, root.createSVGMatrix().translate(p.x - stateOrigin.x, p.y - stateOrigin.y).multiply(g.getCTM().inverse()).multiply(stateTarget.getCTM()));

		stateOrigin = p;
	}
}

/**
 * Handle click event.
 */
function handleMouseDown(evt) {
	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	var g = svgDoc.getElementById("viewport");

	if(true || evt.target.tagName == "svg") {
		// Pan mode
		state = 'pan';

		stateTf = g.getCTM().inverse();

		stateOrigin = getEventPoint(evt).matrixTransform(stateTf);
	} else {
		// Move mode
		state = 'move';

		stateTarget = evt.target;

		stateTf = g.getCTM().inverse();

		stateOrigin = getEventPoint(evt).matrixTransform(stateTf);
	}
}

/**
 * Handle mouse button release event.
 */
function handleMouseUp(evt) {
	if(evt.preventDefault)
		evt.preventDefault();

	evt.returnValue = false;

	var svgDoc = evt.target.ownerDocument;

	if(state == 'pan' || state == 'move') {
		// Quit pan mode
		state = '';
	}
}

]]></script>
`
