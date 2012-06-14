package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

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

func main() {
	var info Info
	err := json.NewDecoder(os.Stdin).Decode(&info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode stats: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("total %v\n", info.Total)
	latency(info.Stats)
	connect(info.Stats)
	delay(info.Stats)
	errors(info.Stats)
}

func latency(stats []Stat) {
	var ds durations
	for _, s := range stats {
		for _, l := range s.Latency {
			ds.add(l)
		}
	}
	ds.prepare()
	fmt.Printf("latency: min %v; max %v; mean %v; median %v\n", ds.min(), ds.max(), ds.mean(), ds.median())
}

func errors(stats []Stat) {
	n := 0
	msgs := make(map[string]bool)
	for _, s := range stats {
		if s.Error != "" {
			n++
			msgs[s.Error] = true
		}
	}
	if n > 0 {
		fmt.Printf("%d errors\n", n)
		for m := range msgs {
			fmt.Printf("\t%s\n", m)
		}
	}
}

func connect(stats []Stat) {
	var ds durations
	for _, s := range stats {
		ds.add(s.Connect)
	}
	ds.prepare()
	fmt.Printf("connect: min %v; max %v; mean %v; median %v\n", ds.min(), ds.max(), ds.mean(), ds.median())
}

func delay(stats []Stat) {
	var ds durations
	for _, s := range stats {
		ds.add(s.Delay)
	}
	ds.prepare()
	fmt.Printf("delay: min %v; max %v; mean %v; median %v\n", ds.min(), ds.max(), ds.mean(), ds.median())
}

type durations []time.Duration

func (ds *durations) add(d time.Duration) {
	*ds = append(*ds, d)
}

func (ds durations) prepare() {
	sort.Sort(ds)
}

func (ds durations) Less(i, j int) bool { return ds[i] < ds[j] }

func (ds durations) Swap(i, j int) { ds[i], ds[j] = ds[j], ds[i] }

func (ds durations) Len() int { return len(ds) }

func (ds durations) sum() time.Duration {
	tot := time.Duration(0)
	for _, d := range ds {
		tot += d
	}
	return tot
}

func (ds durations) mean() time.Duration {
	if len(ds) == 0 {
		return 0
	}
	return ds.sum() / time.Duration(len(ds))
}

func (ds durations) max() time.Duration {
	if len(ds) == 0 {
		return 0
	}
	return ds[len(ds)-1]
}

func (ds durations) min() time.Duration {
	if len(ds) == 0 {
		return 0
	}
	return ds[0]
}

func (ds durations) median() time.Duration {
	if len(ds) == 0 {
		return 0
	}
	return ds[len(ds)/2]
}
