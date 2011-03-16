#include <u.h>
#include <libc.h>
#include <fcall.h>
#include <9pclient.h>
#include <auth.h>
#include <thread.h>

void
threadmain(int argc, char **argv)
{
	char *id;
	CFsys *fs;
	CFid *addr, *ctl;
	char buf[100];
	int n;

	id = getenv("winid");
	if(id == nil){
		sysfatal("acmedot: not run inside acme window");
	}
	fs = nsamount("acme", nil);
	if(fs == nil){
		sysfatal("acmedot: %r");
	}

	snprint(buf, sizeof(buf), "acme/%s/addr", id);
	addr = fsopen(fs, buf, OREAD);
	if(addr == nil){
		sysfatal("acmedot: cannot open %s: %r", buf);
	}

	snprint(buf, sizeof(buf), "acme/%s/ctl", id);
	ctl = fsopen(fs, buf, ORDWR);
	if(ctl == nil){
		sysfatal("acmedot: cannot open %s: %r", buf);
	}

	strcpy(buf, "addr=dot");
	if(fswrite(ctl, buf, strlen(buf)) != strlen(buf)){
		sysfatal("acmedot: cannot set addr: %r");
	}

	n = fsread(addr, buf, sizeof(buf));
	if(n < 0){
		sysfatal("acmedot: cannot read addr: %r");
	}
	fsclose(ctl);
	fsclose(addr);
	fsunmount(fs);

	write(1, buf, n);
}
