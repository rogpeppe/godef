/*
Calc is a calculator designed to be run on the command line.
Expressions rarely require quoting and work easily
with other command line tools that produce or require
whitespace-separated text.

Here is a brief overview by demonstration:

	# The command line is revert-polish - operands are
	# pushed onto a stack; operators pop them off and
	# push the result(s).
	% calc 3 4 5 pow pow
	373391848741020043532959754184866588225409776783734007750636931722079040617265251229993688938803977220468765065431475158108727054592160858581351336982809187314191748594262580938807019951956404285571818041046681288797402925517668012340617298396574731619152386723046235125934896058590588284654793540505936202376547807442730582144527058988756251452817793413352141920744623027518729185432862375737063985485319476416926263819972887006907013899256524297198527698749274196276811060702333710356481
	% 
	% # The default type for a literal without a decimal point is a big integer.
	% # Conversion operators (float, big, rat) can convert from one to another.
	% calc 3 4 5 pow pow float
	-7.087363323575677e+18
	% 
	% # A sequence of operations enclosed in [ ] is executed
	% # repeatedly until the stack is empty.
	% calc 3 6 8 10 [ + ]
	27
	% 
	% # x is used for multiplication to avoid the
	% # need to quote it in the shell.
	% calc 2 3 4 [ x ]
	24
	% 
	% # If there's more than one value left on the stack,
	% # they're all printed.
	% calc 5 7 8 10 +
	5
	7
	18
	% 
	% # A sequence of operations enclosed in [[ ]] is executed
	% # for every item (or set of items, if it uses more
	% # than one) in the stack.
	% calc 150 256 645 [[ 10 / ]]
	15
	25
	64
	% 
	% # We can nest sequences. Within a sequence,
	% # a repetition operator only goes back to the
	% # start of the sequence. (BUG here)
	% calc 3 4 5 [[ 2 3 4 5 [ + ] x ]]
	126
	70
	%  calc 5 6 7 8 1 2 [[ + ]]
	11
	15
	3
	% 
	% # Operations are on integers by default...
	% calc 5 13 /
	0
	% 
	% # but can be converted to rationals. In any
	% # operation, the first operand (deepest in the stack)
	% # determines the result. Other operands are
	% # converted to its type.
	% calc 5 rat 13 /
	5/13
	% calc 5 rat 13 / 2/7 +
	61/91
	% 
	% # The % operator can be used to determine an output format.
	% calc 3 5 pow %x
	f3
	% 
	% # Any format acceptable to Go's fmt.Print may be used.
	% calc 3 5 pow '%#7.5x'
	0x000f3
	% 
	% # The % format operator "taints" the top value on the stack
	% # with the format - any operation on that value
	% # will taint the result too.
	% calc 3 '%6d' 5 pow
	   243
	% 
	% # If the % format operator is executed on an empty stack,
	% # it sets the default format for the stack.
	% calc %o 10 11 12
	12
	13
	14
	% 
*/
package documentation
