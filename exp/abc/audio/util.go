package audio
import (
	"bytes"
	"fmt"
)

var indentLevel int

func un(_ bool, rets ... interface{}) {
	if x := recover(); x != nil {
		panic(x)
	}
	indentLevel--
	if Debug {
		s := ""
		if len(rets) > 0 {
			s = " -> " + fmt.Sprint(rets...)
		}
		fmt.Printf("%s}%s\n", indent(), s)
	}
}

func log(f string, args ... interface{}) bool {
	if Debug {
		if len(f) > 0 && f[len(f) - 1] == '\n' {
			f = f[0:len(f) - 1]
		}
		fmt.Printf("%s%s {\n", indent(), fmt.Sprintf(f, args...))
	}
	indentLevel++
	return true
}

func indent() string {
	var b bytes.Buffer
	for i := 0; i < indentLevel; i++ {
		b.WriteByte('\t')
	}
	return b.String()
}

var Debug = false
func debugp(f string, a ... interface{}) {
	if Debug {
		if len(f) > 0 && f[len(f) - 1] == '\n' {
			f = f[0:len(f) - 1]
		}
		fmt.Printf("%s%s\n", indent(), fmt.Sprintf(f, a...))
	}
}
