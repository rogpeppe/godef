// Share is a piece of demo code to illustrate the flexibility of the rpc and netchan
// packages. It requires one instance to be running in server mode on some network
// address addr, e.g. localhost:3456:
//
// 	share -s localhost:3456
//
// Then in other windows or on other machines, run some client instances:
//	share -name foo localhost:3456
//
// The name must be different for each client instance.
// When a client instance is running, there are two commands:
//	list
//		List all currently connected client names
//	read client filename
//		Ask the given client for the contents of the named file.
package documentation
