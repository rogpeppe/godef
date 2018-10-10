package broken

import "fmt"

func unclosedIf() {
	if BadExpr {
		var myUnclosedIf string               //@myUnclosedIf
		fmt.Println("s = %v\n", myUnclosedIf) //@godef("my", myUnclosedIf)
}

