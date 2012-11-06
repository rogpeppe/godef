package test
import "other"

func %test test X func+%X() {
	%test test x var+%x := 0
	for %test test i var+%i := 0; %test test i var%i < %test test x var%x; %test test i var%i++ {
		%test test i var+%i := %test test i var%i
		%test test x var%x += %test test i var%i
	}
	other.%test other Println func%Println(%test test x var%x, %test test i var+%i)
	other.%test other Var var%Var = 5
}

type %test test Foo type+%Foo struct{}

func (%test test Foo type%Foo) %test test Foo.Bar func+%Bar() {}
