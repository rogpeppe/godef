// ulimit -n 30000
package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

func echoServer(ws *websocket.Conn) {
	io.Copy(ws, ws)
}

type Stat struct {
	Delay   time.Duration
	Connect time.Duration
	Latency []time.Duration
	Error   string `json:"omitempty"`
}

type Info struct {
	Stats []Stat
	Total time.Duration
}

var stop = make(chan struct{})
var wg sync.WaitGroup

func main() {
	go server()

	info := &Info{Stats: make([]Stat, 10000)}
	t0 := time.Now()
	tprev := t0
	for i := range info.Stats {
		wg.Add(1)
		tnow := time.Now()
		info.Stats[i].Delay = tnow.Sub(tprev)
		tprev = tnow
		go connect(time.Now(), &info.Stats[i])
		time.Sleep(1 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
	info.Total = time.Since(t0)
	data, err := json.MarshalIndent(info, "", "\t")
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(data)
}

func connect(start time.Time, s *Stat) {
	defer wg.Done()
	c, err := websocket.Dial("ws://localhost:8000/", "", "http://localhost/")
	if err != nil {
		s.Error = "dial: " + err.Error()
		return
	}
	in := make([]byte, 3)
	out := make([]byte, len(in))
	s.Connect = time.Since(start)
	for i := 0; i < 60; i++ {
		t0 := time.Now()
		out = strconv.AppendInt(out[:0], int64(i), 10)
		n, err := c.Write(out)
		if err != nil {
			s.Error = "write: " + err.Error()
			return
		}
		if n != len(out) {
			s.Error = fmt.Sprintf("unexpected write count %d/%d", n, len(out))
			return
		}
		n, err = c.Read(in)
		t1 := time.Now()
		if err != nil {
			s.Error = "read: " + err.Error()
			return
		}
		if n != len(out) {
			s.Error = fmt.Sprintf("unexpected read data %q", in[0:n])
			return
		}
		if !bytes.Equal(out, in[0:n]) {
			s.Error = fmt.Sprintf("unexpected ping result, expect %q got %q", out, in[0:n])
			return
		}
		s.Latency = append(s.Latency, t1.Sub(t0))
		time.Sleep(1 * time.Second)
		select {
		case <-stop:
			return
		default:
		}
	}
}

func server() {
	http.Handle("/", websocket.Handler(echoServer))

	if err := http.ListenAndServe(":8000", nil); err != nil {
		log.Fatalf("http.ListenAndServe: %v", err)
	}
}
