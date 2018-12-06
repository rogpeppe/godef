package print

import (
	"github.com/rogpeppe/godef/a" //@mark(PrintImportDir, "rogpeppe")
	"github.com/rogpeppe/godef/b"
)

type localStruct struct {
	Exported bool
	private  bool
}

func printing() {
start:
	var thing localStruct
	if thing.private {
		thing.Exported = false
		goto start //@mark(PrintStart, "start")
	}
	a.Stuff()    //@mark(PrintA, "a"),mark(PrintStuff, "Stuff")
	var _ = b.S1 //@mark(PrintS1, "S1")
	const c1 = 5
	if c1 == 2 { //@mark(PrintC1, "c1")
	}

	/*@
	godefPrint(PrintImportDir, "json", re`godef[/\\]a\s*$`)

	godefPrint(PrintA, "json", re`^(|
		){"filename":".*godef.print.print\.go","line":\d+,"column":\d+}\n$`)
	godefPrint(PrintA, "type", re`^(|
		).*godef.print.print\.go:\d+:\d+(\n|
		)import \(a "github\.com/rogpeppe/godef/a"\)\n$`)

	godefPrint(PrintStuff, "json", re`^(|
		){"filename":".*godef.a.a\.go","line":\d+,"column":\d+}\n$`)
	godefPrint(PrintStuff, "type", re`^(|
		).*godef.a.a\.go:\d+:\d+(\n|
		).*Stuff func\(\)\n$`)
	godefPrint(PrintStuff, "public", re`^(|
		).*godef.a.a\.go:\d+:\d+(\n|
		).*Stuff func\(\)\n$`)
	godefPrint(PrintStuff, "all", re`^(|
		).*godef.a.a\.go:\d+:\d+(\n|
		).*Stuff func\(\)\n$`)

	godefPrint(PrintC1, "type", re`^(|
		).*godef.print.print\.go:\d+:\d+(\n|
		)const c1 (untyped )?int = 5\n$`)

	godefPrint(PrintStart, "type", re`^(|
		).*godef.print.print\.go:\d+:\d+(\n|
		)label start\n$`)

	godefPrint(PrintS1, "json", re`^(|
		){"filename":".*godef.b.b\.go","line":\d+,"column":\d+}\n$`)
	godefPrint(PrintS1, "type", re`^(|
		).*godef.b.b\.go:\d+:\d+(\n|
		)type S1 struct\s*\{\s*F1\s+int[\n;]\s*f2\s+int[\n;]\s*f3\s+S2[\n;]\s*S2\s*\}\n$`)
	// this succeeds, but lists no fields which seems wrong
	_godefPrint(PrintS1, "public", re`^(|
		).*godef.b.b\.go:\d+:\d+(\n|
		)type S1 struct\s*\{\s*F1\s+int[\n;]\s*f2\s+int[\n;]\s*f3\s+S2[\n;]\s*S2\s*\}\n$`)
	// the following fails, but it lists F1 twice, once as 'F1 string' which is wrong
	_godefPrint(PrintS1, "all", re`^(|
		).*godef.b.b\.go:\d+:\d+(\n|
		)type S1 struct\s*\{\s*F1\s+int[\n;]\s*f2\s+int[\n;]\s*f3\s+S2[\n;]\s*S2\s*\}\n$`)
	*/
}
