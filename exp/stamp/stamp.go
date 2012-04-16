// Timestamp recording (for debugging).
package stamp

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
	"time"
)

type stamp struct {
	msg string
	t   int64
}

var mu sync.Mutex

type stampVector []stamp

var stamps = make(stampVector, 0, 100)

// AddTime records a timestamp at the given time.
func AddTime(msg string, t int64) {
	mu.Lock()
	stamps = append(stamps, stamp{msg, t})
	mu.Unlock()
}

// Add records a timestamp at the current time.
func Add(msg string) {
	AddTime(msg, time.Now())
}

// String returns a textual representation of all the time stamps with
// their associated messages.
func String() string {
	mu.Lock()
	defer mu.Unlock()
	if len(stamps) == 0 {
		return "no stamps"
	}
	var buf bytes.Buffer
	sort.Sort(stamps)
	t0 := stamps[0].t
	fmt.Fprintf(&buf, "start: %d", t0)
	for _, s := range stamps {
		fmt.Fprintf(&buf, "; %d %s", s.t-t0, s.msg)
	}
	return buf.String()
}

func (stampVector) Len() int {
	return len(stamps)
}

func (stampVector) Swap(i, j int) {
	stamps[i], stamps[j] = stamps[j], stamps[i]
}

func (stampVector) Less(i, j int) bool {
	return stamps[i].t < stamps[j].t
}
