// The stackgraph command reads a Go stack trace (as produced by a Go
// panic) from its standard input and writes an SVG file suitable for
// viewing in a web browser on its standard output. It assumes that
// graphviz is installed.
//
// All the dot(1) heuristics were unashamedly stolen from go tool pprof.
//
// Example
//
//	$ stackgraph < file-containing-go-panic > /tmp/x.svg
//	# open web browser on file:///tmp/x.svg
package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
)

type Node struct {
	Func     string
	Id       int
	Count    int
	ArgCount []map[uint64]int
	called   map[int]bool
}

// Arc represents a call from node 0 to node 1.
type Arc struct {
	Node0, Node1 *Node
}

type Edge struct {
	Count int
}

func main() {
	stacks, err := parseStacks(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	nodeId := 0
	nodes := make(map[string]*Node)
	totalCalls := 0
	for _, stack := range stacks {
		for _, call := range stack.Calls {
			node := nodes[call.Func]
			if node == nil {
				node = &Node{
					Func:   call.Func,
					Id:     nodeId,
					called: make(map[int]bool),
				}
				nodes[call.Func] = node
				nodeId++
			}
			// Don't increment the count twice for a function
			// that appears more than once in the call stack.
			if !node.called[stack.Goroutine] {
				node.Count++
				totalCalls++
				for i, arg := range call.Args {
					if i == len(node.ArgCount) {
						node.ArgCount = append(node.ArgCount, make(map[uint64]int))
					}
					node.ArgCount[i][arg]++
				}
			}
		}
	}
	totalEdges := 0
	edges := make(map[Arc]*Edge)
	for _, stack := range stacks {
		prevNode := nodes[stack.Calls[0].Func]
		for _, call := range stack.Calls[1:] {
			node := nodes[call.Func]
			arc := Arc{node, prevNode}
			edge := edges[arc]
			if edge == nil {
				edge = &Edge{}
				edges[arc] = edge
			}
			edge.Count++
			totalEdges++
			prevNode = node
		}
	}
	if err := writeSVG(os.Stdout, &Summary{
		Title:      fmt.Sprintf("%d total goroutines", len(stacks)),
		Edges:      edges,
		Nodes:      nodes,
		TotalCalls: totalCalls,
		TotalEdges: totalEdges,
	}); err != nil {
		log.Fatalf("cannot write SVG: %v", err)
	}
}

type Call struct {
	Func   string
	Source string
	Args   []uint64
}

type Stack struct {
	Goroutine int
	Calls     []Call
}

func parseStacks(r io.Reader) ([]*Stack, error) {
	var stacks []*Stack
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		stack := &Stack{}
		if n, _ := fmt.Sscanf(line, "goroutine %d", &stack.Goroutine); n != 1 {
			continue
		}
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				// empty line signifies end of a stack
				break
			}
			if strings.Contains(line, "  ") {
				// Looks like a register dump.
				// TODO better heuristic here.
				continue
			}
			var call Call
			if strings.HasSuffix(line, ")") {
				if i := strings.LastIndex(line, "("); i > 0 {
					call.Args = parseArgs(line[i+1 : len(line)-1])
					line = line[0:i]
				}
			}
			call.Func = strings.TrimPrefix(line, "created by ")
			if !scanner.Scan() {
				break
			}
			line = scanner.Text()
			if strings.HasPrefix(line, "\t") {
				line = strings.TrimPrefix(line, "\t")
				if i := strings.LastIndex(line, " +"); i >= 0 {
					line = line[0:i]
				}
				call.Source = line
			}
			stack.Calls = append(stack.Calls, call)
		}
		if len(stack.Calls) > 0 {
			stacks = append(stacks, stack)
		}
	}
	return stacks, nil
}

func parseArgs(argList string) []uint64 {
	argList = strings.TrimSuffix(argList, ", ...")
	if argList == "" {
		return nil
	}
	parts := strings.Split(argList, ", ")
	args := make([]uint64, len(parts))
	for i, a := range parts {
		n, err := strconv.ParseUint(a, 0, 64)
		if err != nil {
			panic(fmt.Errorf("cannot parse %q (from %q)", a, argList))
			n = 0xdeadbeef
		}
		args[i] = n
	}
	return args
}
