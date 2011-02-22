implement Styxlisten;
include "sys.m";
	sys: Sys;
include "draw.m";
include "keyring.m";
	keyring: Keyring;
include "security.m";
	auth: Auth;
include "styx.m";
	styx: Styx;
	Tmsg, Rmsg, NOFID, NOTAG: import styx;
include "arg.m";
include "sh.m";

Styxlisten: module {
	init: fn(nil: ref Draw->Context, argv: list of string);
};

logfd: ref Sys->FD;
log(s: string) {
   sys->fprint(logfd, "%s\n", s);
}

badmodule(p: string)
{
	sys->fprint(stderr(), "styxlisten: cannot load %s: %r\n", p);
	raise "fail:bad module";
}

verbose := 0;
passhostnames := 0;

Maxmsgsize: con 65536;

NFIDHASH: con 17;
Client: adt {
	fids:  array of list of ref Fid;
	fd: ref Sys->FD;
	uname: string;
	hostname: string;
	readonly: int;
	msize: int;

	reply: fn(c: self ref Client, m: ref Rmsg);
	getfid: fn(c: self ref Client, fid: int): ref Fid;
	newfid: fn(c: self ref Client, cfid: int, seq: int): ref Fid;
	gettag: fn(c: self ref Client, tag: int): ref Tag;
	delfid: fn(c: self ref Client, fid: ref Fid);
};

Tag: adt {
	client: ref Client;
	stag: int;
	ctag: int;
	req: ref Req;
	seq: ref Seq;
	next: cyclic ref Tag;
	seqclose: fn(t: self ref Tag, ok: int);
	flushreq: fn(t: self ref Tag, req: ref Req);
	sendreq: fn(t: self ref Tag, req: ref Req);
};

Tagmap: adt {
	tags: array of ref Tag;
};

Fid: adt {
	cfid: int;
	sfid: int;
	pending: int;
	seqstag: int;			# server tag of sequence; >= 0 if attached to a Seq.
};

Seq: adt {
	client: ref Client;
	fids: list of ref Fid;		# fids to be clunked if seq fails.
	h, t: ref Req;
	tEOF: int;
	rEOF: int;
	err: int;

	addfid: fn(s: self ref Seq, fid: ref Fid);
	delfid: fn(s: self ref Seq, fid: ref Fid);
	put: fn(s: self ref Seq, m: ref Req);
	get: fn(s: self ref Seq): ref Req;
};
blankseq: Seq;

None, Del, New, Nonseq: con iota;

Req: adt {
	tmsg: ref Tmsg;
	fid: ref Fid;		# only used if we need to perform some action on reply.
	action: int;		# action to be performed on fid. (Del, New or Nonseq)
	aborterror: string;	# actual error message to deliver.
	next: cyclic ref Req;
};

freetags: ref Tag;
maxtag := 100;
tags: ref Tag;

freefids: list of int;
maxfid := 200;
queryreadonly: chan of (string, chan of int);

srvfd: ref Sys->FD;

init(ctxt: ref Draw->Context, argv: list of string)
{
	sys = load Sys Sys->PATH;
	auth = load Auth Auth->PATH;
	if (auth == nil)
		badmodule(Auth->PATH);
	if ((e := auth->init()) != nil)
		error("auth init failed: " + e);
	keyring = load Keyring Keyring->PATH;
	if (keyring == nil)
		badmodule(Keyring->PATH);
	styx = load Styx Styx->PATH;
	if(styx == nil)
		badmodule(Styx->PATH);
	styx->init();

	arg := load Arg Arg->PATH;
	if (arg == nil)
		badmodule(Arg->PATH);

	arg->init(argv);
	arg->setusage("styxlisten [-a alg]... [-hAtsv] [-U usersfile] [-k keyfile] address cmd [arg...]");

	algs: list of string;
	doauth := 1;
	synchronous := 0;
	trusted := 0;
	keyfile := "";
	adminuserfile := "";

	while ((opt := arg->opt()) != 0) {
		case opt {
		'v' =>
			verbose = 1;
		'a' =>
			algs = arg->earg() :: algs;
		'f' or
		'k' =>
			keyfile = arg->earg();
			if (! (keyfile[0] == '/' || (len keyfile > 2 &&  keyfile[0:2] == "./")))
				keyfile = "/usr/" + user() + "/keyring/" + keyfile;
		'h' =>
			passhostnames = 1;
		't' =>
			trusted = 1;
		'U' =>
			adminuserfile = arg->earg();
		's' =>
			synchronous = 1;
		'A' =>
			doauth = 0;
		* =>
			arg->usage();
		}
	}
	argv = arg->argv();
	if (len argv < 2)
		arg->usage();
	arg = nil;
	if (doauth && algs == nil)
		algs = getalgs();
	addr := netmkaddr(hd argv, "tcp", "styx");
	cmd := tl argv;
logfd = sys->create("/tmp/styxlisten.log", Sys->OWRITE, 8r666);
log("************ start");

	authinfo: ref Keyring->Authinfo;
	if (doauth) {
		if (keyfile == nil)
			keyfile = "/usr/" + user() + "/keyring/default";
		authinfo = keyring->readauthinfo(keyfile);
		if (authinfo == nil)
			error(sys->sprint("cannot read %s: %r", keyfile));
	}

	(ok, c) := sys->announce(addr);
	if (ok == -1)
		error(sys->sprint("cannot announce on %s: %r", addr));
	if(!trusted){
		sys->unmount(nil, "/mnt/keys");	# should do for now
		# become none?
	}
	if(adminuserfile != nil)
		spawn adminproc(adminuserfile, queryreadonly = chan of (string, chan of int));

	lsync := chan[1] of int;
	srvfd = popen(ctxt, cmd, lsync);
	if(synchronous)
		listener(c, authinfo, algs, lsync);
	else
		spawn listener(c, authinfo, algs, lsync);
}

adminproc(f: string, query: chan of (string, chan of int))
{
	fd: ref Sys->FD;
	stat: ref Sys->Dir;
	users: list of string;
	for(;;){
		(user, reply) := <-query;
		if(fd == nil && (fd = sys->open(f, Sys->OREAD)) == nil){
			reply <-= 1;
			continue;
		}
		(ok, nstat) := sys->fstat(fd);
		if(ok == -1){
			reply <-= 1;
			continue;
		}
		if(stat == nil ||
				nstat.qid.path != stat.qid.path ||
				nstat.qid.vers != stat.qid.vers ||
				nstat.mtime != stat.mtime){
			users = readusers(fd);
		}
		stat = ref nstat;
		for(u := users; u != nil; u = tl u)
			if(user == hd u)
				break;
		reply <-= u == nil;
	}
}

readusers(fd: ref Sys->FD): list of string
{
	sys->seek(fd, big 0, Sys->SEEKSTART);
	buf := array[Sys->ATOMICIO] of byte;
	# XXX 8K limit on users file!
	n := sys->read(fd, buf, len buf);
	return sys->tokenize(string buf[0:n], " \t\n").t1;
}

msgsize: int;

listener(c: Sys->Connection, authinfo: ref Keyring->Authinfo, algs: list of string, lsync: chan of int)
{
	lsync <-= sys->pctl(0, nil);

	version();
	tc := chan of (ref Tmsg, ref Client);
	rc := chan of ref Rmsg;
	spawn rmsgreader(rc);
	spawn central(tc, rc);

	for (;;) {
		(n, nc) := sys->listen(c);
		if (n == -1)
			error(sys->sprint("listen failed: %r"));
		if (verbose)
			sys->fprint(stderr(), "styxlisten: got connection from %s",
					readfile(nc.dir + "/remote"));
		dfd := sys->open(nc.dir + "/data", Sys->ORDWR);
		if (dfd != nil) {
			if(nc.cfd != nil)
				sys->fprint(nc.cfd, "keepalive");
			hostname := readfile(nc.dir + "/remote");
			if(hostname != nil) {
				hostname = hostname[0:len hostname - 1];
			}
log(sys->sprint("------ conn %s", hostname));
			if (algs == nil) {
				spawn tmsgread(tc, nil, hostname, dfd);
			} else
				spawn authenticator(dfd, authinfo, tc, algs, hostname);
		}
	}
}

# authenticate a connection and set the user id.
authenticator(dfd: ref Sys->FD, authinfo: ref Keyring->Authinfo,
		tc: chan of (ref Tmsg, ref Client),
		algs: list of string, hostname: string)
{
	# authenticate and change user id appropriately
	(fd, err) := auth->server(algs, authinfo, dfd, 1);
	if (fd == nil) {
		if (verbose)
			sys->fprint(stderr(), "styxlisten: authentication failed: %s\n", err);
		return;
	}
	if (verbose)
		sys->fprint(stderr(), "styxlisten: client authenticated as %s\n", err);
	spawn tmsgread(tc, err, hostname, dfd);
}

error(e: string)
{
	sys->fprint(stderr(), "styxlisten: %s\n", e);
	raise "fail:error";
}
	
popen(ctxt: ref Draw->Context, argv: list of string, lsync: chan of int): ref Sys->FD
{
	sync := chan of int;
	fds := array[2] of ref Sys->FD;
	sys->pipe(fds);
	spawn runcmd(ctxt, argv, fds[0], sync, lsync);
	<-sync;
	return fds[1];
}

runcmd(ctxt: ref Draw->Context, argv: list of string, stdin: ref Sys->FD,
		sync: chan of int, lsync: chan of int)
{
	sys->pctl(Sys->FORKFD, nil);
	sys->dup(stdin.fd, 0);
	stdin = nil;
	sync <-= 0;
	sh := load Sh Sh->PATH;
	e := sh->run(ctxt, argv);
	kill(<-lsync, "kill");		# kill listener, as command has exited
	if(verbose){
		if(e != nil)
			sys->fprint(stderr(), "styxlisten: command exited with error: %s\n", e);
		else
			sys->fprint(stderr(), "styxlisten: command exited\n");
	}
}

kill(pid: int, how: string)
{
	sys->fprint(sys->open("/prog/"+string pid+"/ctl", Sys->OWRITE), "%s", how);
}

user(): string
{
	if ((s := readfile("/dev/user")) == nil)
		return "none";
	return s;
}

readfile(f: string): string
{
	fd := sys->open(f, sys->OREAD);
	if(fd == nil)
		return nil;

	buf := array[1024] of byte;
	n := sys->read(fd, buf, len buf);
	if(n < 0)
		return nil;

	return string buf[0:n];	
}

getalgs(): list of string
{
	sslctl := readfile("#D/clone");
	if (sslctl == nil) {
		sslctl = readfile("#D/ssl/clone");
		if (sslctl == nil)
			return nil;
		sslctl = "#D/ssl/" + sslctl;
	} else
		sslctl = "#D/" + sslctl;
	(nil, algs) := sys->tokenize(readfile(sslctl + "/encalgs") + " " + readfile(sslctl + "/hashalgs"), " \t\n");
	return "none" :: algs;
}

stderr(): ref Sys->FD
{
	return sys->fildes(2);
}

netmkaddr(addr, net, svc: string): string
{
	if(net == nil)
		net = "net";
	(n, nil) := sys->tokenize(addr, "!");
	if(n <= 1){
		if(svc== nil)
			return sys->sprint("%s!%s", net, addr);
		return sys->sprint("%s!%s!%s", net, addr, svc);
	}
	if(svc == nil || n > 2)
		return addr;
	return sys->sprint("%s!%s", addr, svc);
}

tmsgread(tc: chan of (ref Tmsg, ref Client),
	uname, hostname: string, dfd: ref Sys->FD)
{
	client := ref Client(
		array[NFIDHASH] of list of ref Fid,
		dfd,
		uname,
		hostname,
		0,
		0
	);
	if(queryreadonly != nil){
		queryreadonly <-= (uname, c := chan of int);
		client.readonly = <-c;
	}
	# do version negotiation before handing all messages off to central.
	tm := Tmsg.read(dfd, 0);
log(sys->sprint("%s <- %s", tm.text(), client.hostname));
	pick t := tm {
	Version =>
		v: string;
		(client.msize, v) = styx->compatible(t, msgsize, nil);
		if(v == "unknown"){
			client.reply(ref Rmsg.Error(t.tag, "unknown version"));
			return;
		}
		client.reply(ref Rmsg.Version(t.tag, client.msize, v));
	* =>
		client.reply(ref Rmsg.Error(t.tag, "expected Tversion, got "+t.text()));
		return;
	}
	while((t := Tmsg.read(dfd, client.msize)) != nil && tagof t != tagof Tmsg.Readerror){
		tc <-= (t, client);
	}
	if(t != nil){
		pick r := t {
		Readerror =>
			sys->fprint(stderr(), "styxlisten: tmsg read error: %s\n", r.error);
		}
	}
	tc <-= (nil, client);
}

iswritemsg(gm: ref Tmsg): int
{
	pick m := gm {
	Wstat =>
		return 1;
	Write =>
		return 1;
	Create =>
		return 1;
	Open =>
		return m.mode != Sys->OREAD;
	Remove =>
		return 1;
	}
	return 0;
}

rmsgreader(rc: chan of ref Rmsg)
{
	while((r := Rmsg.read(srvfd, msgsize)) != nil && tagof r != tagof Rmsg.Readerror)
		rc <-= r;
	if(r != nil){
		pick t := r {
		Readerror =>
			sys->fprint(stderr(), "styxlisten: rmsg read error: %s\n", t.error);
		}
	}
	rc <-= nil;
}

writetmsg(tm: ref Tmsg) {
log(sys->sprint("\t%s", tm.text()));
	d := tm.pack();
	sys->write(srvfd, d, len d);
}

# client c is dead. flush all its outstanding tags, clunk all its extant fids.
deadclient(c: ref Client)
{
log(sys->sprint("------ dead client %s", c.hostname));
	for(t := tags; t != nil; t = t.next){
		if(t.client == c){
			t.client = nil;
			ft := newtag(-1);
			ft.req = newreq(ref Tmsg.Flush(ft.stag, t.stag));
			writetmsg(ft.req.tmsg);
			if(t.seq != nil){
				t.seq.client = nil;
				# terminate the sequence. we're not guaranteed that
				# the above flush request will be processed before
				# the sequence end, so abort the sequence too.
				t.sendreq(newreq(ref Tmsg.Flush(-1, t.stag)));
				t.sendreq(newreq(ref Tmsg.End(-1)));
			}
		}
	}
	for(i := 0; i < len c.fids; i++){
		for(fl := c.fids[i]; fl != nil; fl = tl fl){
			fid := hd fl;
			# if there's a pending request to create the fid,
			# then we'll send a clunk request when the reply
			# arrives, unless the flush reply arrives first.
			# likewise, if the fid is part of a sequence, it
			# will be clunked when the sequence is flushed
			# or if the sequence terminates, whichever happens first.
			if(!fid.pending && fid.seqstag == -1){
				localclunk(fid);
			}
		}
	}
	c.fids = nil;
log("------ client disposed");
}

localclunk(fid: ref Fid)
{
	req := newreq(ref Tmsg.Clunk(-1, fid.sfid));
	req.fid = fid;
	req.action = Del;
	newtag(-1).sendreq(req);
}

blankreq: Req;
newreq(tm: ref Tmsg): ref Req
{
	req := ref blankreq;
	req.tmsg = tm;
	return req;
}

Tag.sendreq(t: self ref Tag, req: ref Req)
{
	if(t.seq != nil){
		t.seq.put(req);
	}else{
		t.req = req;
	}
	req.tmsg.tag = t.stag;
	writetmsg(req.tmsg);
}

version() {
	# negotiate version, with large message size (clients are free to negotiate
	# a smaller message size if they wish)
	# TODO: change version id.
	writetmsg(ref Tmsg.Version(NOTAG, Maxmsgsize, "9P2000"));
	rmsg := Rmsg.read(srvfd, 0);
	pick r := rmsg {
	Error =>
		error(sys->sprint("version negotiation failed: %s", r.ename));
	Version =>
		msgsize = r.msize;
	Readerror =>
		error(sys->sprint("read error: %s", r.error));
	* =>
		error(sys->sprint("unexpected version reply: %s", rmsg.text()));
	}
}

central(tc: chan of (ref Tmsg, ref Client), rc: chan of ref Rmsg)
{
	for(;;) alt {
	(tmsg, c) := <-tc =>
		if(tmsg == nil){
			deadclient(c);
			continue;
		}
log(sys->sprint("%s <- %s", tmsg.text(), c.hostname));
		t := c.gettag(tmsg.tag);
		if(t != nil && t.seq == nil){
			t.client.reply(ref Rmsg.Error(tmsg.tag, "duplicate tag"));
			break;
		}
		req := newreq(tmsg);
		if(c.readonly && iswritemsg(tmsg)){
			if(t == nil){
				t.client.reply(ref Rmsg.Error(tmsg.tag, "read-only filesystem"));
				break;
			}
			req.aborterror = "read-only filesystem";
		}
		seqstag := -1;
		if(t == nil){
			t = newtag(tmsg.tag);
			t.client = c;
		}else{
			seqstag = t.stag;
		}
		opfid: ref Fid;
		pick m := tmsg {
		Auth =>
			req.fid = c.newfid(m.afid, seqstag);
			req.action = New;
			m.afid = req.fid.sfid;
		Attach =>
			req.fid = c.newfid(m.fid, seqstag);
			req.action = New;
			if(m.afid != NOFID){
				opfid = c.getfid(m.afid);
				m.afid = opfid.sfid;
			}
			m.fid = req.fid.sfid;
			if(passhostnames)
				m.uname = c.uname + " " + c.hostname;	# XXX quote
			else
				m.uname = c.uname;
		Begin =>
			# TODO: what do we do if it's already a sequence (invalid).
			t.seq = ref blankseq;
			t.seq.client = t.client;
		End =>
			t.seq.tEOF = 1;
			# TODO: what do we do if it's not already a sequence (invalid)
			# perhaps if a client misbehaves we could poison its connection.
			t.seqclose(1);
		Nonseq =>
			# XXX check that fid is actually part of a sequence?
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
			req.fid = opfid;
			req.action = Nonseq;
		Walk =>
			same := m.newfid == m.fid;
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
			if(same){
				m.newfid = m.fid;
			}else{
				req.fid = c.newfid(m.newfid, seqstag);
				req.action = New;
				m.newfid = req.fid.sfid;
			}
		Open =>
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
		Create =>
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
		Read =>
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
		Write =>
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
		Stat =>
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
		Wstat =>
			opfid = c.getfid(m.fid);
			m.fid = opfid.sfid;
		Clunk =>
			opfid = c.getfid(m.fid);
			req.fid = opfid;
			req.action = Del;
			m.fid = opfid.sfid;
		Remove =>
			opfid = c.getfid(m.fid);
			req.fid = opfid;
			req.action = Del;
			m.fid = opfid.sfid;
		}
		if(opfid != nil && opfid.seqstag >= 0 && t.stag != opfid.seqstag){
			panic("bad sequence access, stag " + string t.stag + "; fid seq stag " + string opfid.seqstag);
		}
		
		t.sendreq(req);
showfidmap(c);

	rmsg := <-rc =>
log(sys->sprint("\t%s", rmsg.text()));
		t := getstag(rmsg.tag);
		req := t.req;
		if(t.seq != nil){
			req = t.seq.get();
		}
		case tagof rmsg {
		tagof Rmsg.Walk or
		tagof Rmsg.Error =>
			err := 1;
			# A walk response with less elements than asked for
			# counts as an error.
			pick rm := rmsg {
			Walk =>
				pick tm := req.tmsg {
				Walk =>
					if(len rm.qids == len tm.names){
						err = 0;
					}
				}
			}
			if(!err){
				break;
			}
			if(t.seq != nil){
				t.seq.rEOF = 1;
				t.seqclose(0);
				if(req.aborterror != nil){
					rmsg = ref Rmsg.Error(rmsg.tag, req.aborterror);
				}
			}
			case req.action {
			New =>
				req.action = Del;
			Nonseq =>
				req.action = None;
			}
		tagof Rmsg.End =>
			t.seq.rEOF = 1;
			t.seqclose(1);
		* =>
			pick tm := req.tmsg {
			Flush =>
				if((tag := getstag(tm.oldtag)) != nil) {
					flushtag(tag);
				}
			}
		}
		case req.action {
		Del =>
			if(t.seq != nil){
				t.seq.delfid(req.fid);
			}
			t.client.delfid(req.fid);
log(sys->sprint("\tdeleted %d(%d)", req.fid.cfid, req.fid.sfid));
showfidmap(t.client);
		New =>
			req.fid.pending = 0;
			if(t.client == nil && req.fid.seqstag == -1){
				# if the client has gone away, then
				# we need to do the clunk for them
				# because we couldn't do it at the time.
				# if the fid is attached to a sequence, we'll already
				# have flushed the tag, so the fid will be clunked
				# as a result of that, so we don't do it here.
				localclunk(req.fid);
			}else if(t.seq != nil){
				t.seq.addfid(req.fid);
			}
		Nonseq =>
			if(t.seq != nil){
				t.seq.delfid(req.fid);
				req.fid.seqstag = -1;
			}
		}
		rmsg.tag = t.ctag;
		if(t.client != nil){
			t.client.reply(rmsg);
			if(t.seq == nil){
				deltag(t);
			}
		}else{
			log(sys->sprint("%s -> discard", rmsg.text()));
		}
	}
}

showfidmap(c: ref Client) {
	if(c == nil){
		return;
	}
	s := "\t";
	for(i := 0; i < len c.fids; i++){
		for(fl := c.fids[i]; fl != nil; fl = tl fl){
			f := hd fl;
			s += " " + string f.cfid + "->" + string f.sfid;
			if(f.seqstag >= 0){
				s += "[t"+string f.seqstag+"]";
			}
		}
	}
#	s += "[free ";
#	for(fl := freefids; fl != nil; fl = tl fl){
#		s += string hd fl + " ";
#	}
#	s += "]";
	log(s);
}

flushtag(t: ref Tag) {
	if(t.seq != nil){
		for(req := t.seq.h; req != nil; req = req.next){
			t.flushreq(req);
		}
		t.seq.h = t.seq.t = nil;
		# a sequence being flushed counts as a sequence failing,
		# so we must mark the sequence fids as clunked.
		for(fl := t.seq.fids; fl != nil; fl = tl fl){
			t.client.delfid(hd fl);
		}
		t.seq.fids = nil;
	}else{
		t.flushreq(t.req);
	}
	deltag(t);
}

Tag.flushreq(t: self ref Tag, req: ref Req){
	# if we've got a flush reply and the tag it flushed was
	# creating a new fid, then we need to delete the new fid
	# so it can be reused.
	if(req.action == New){
		t.client.delfid(req.fid);
	}
}

Client.reply(c: self ref Client, m: ref Rmsg) {
log(sys->sprint("%s -> %s", m.text(), c.hostname));
	d := m.pack();
	sys->write(c.fd, d, len d);
}

Tag.seqclose(t: self ref Tag, ok: int) {
	seq := t.seq;
	seq.err = seq.err || !ok;
	if(!seq.tEOF || !seq.rEOF){
		return;
	}
	if(seq.err){
log(sys->sprint("\tsequence c%d[s%d] closed with error, %d fids in seq", t.ctag, t.stag, len(seq.fids)));
		for(fids := seq.fids; fids != nil; fids = tl fids){
			t.client.delfid(hd fids);
		}
showfidmap(t.client);
	}else{
		for(fids := seq.fids; fids != nil; fids = tl fids){
			(hd fids).seqstag = -1;
		}
	}
	t.seq = nil;
}

blanktag: Tag;

newtag(ctag: int): ref Tag
{
	t: ref Tag;
#	if(freetags != nil){
#		(t, freetags) = (freetags, freetags.next);
#	}else{
		t = ref blanktag;
		t.stag = maxtag++;
#	}
	t.ctag = ctag;
	t.next = tags;
	tags = t;
	return t;
}

Client.gettag(c: self ref Client, tag: int): ref Tag
{
	for(t := tags; t != nil; t = t.next){
		if(t.ctag == tag && t.client == c){
			return t;
		}
	}
	return nil;
}

Seq.addfid(seq: self ref Seq, fid: ref Fid) {
	seq.fids = fid :: seq.fids;
}

Seq.delfid(seq: self ref Seq, fid: ref Fid) {
	rfids: list of ref Fid;
	for(fids := seq.fids; fids != nil; fids = tl fids){
		if(hd fids != fid){
			rfids = hd fids :: rfids;
		}
	}
	seq.fids = rfids;
}

Seq.put(seq: self ref Seq, r: ref Req) {
	r.next = nil;
	if(seq.h == nil){
		seq.h = seq.t = r;
		return;
	}
	seq.t.next = r;
	seq.t = r;
}

Seq.get(seq: self ref Seq): ref Req {
	r := seq.h;
	seq.h = seq.h.next;
	if(seq.h == nil){
		seq.t = nil;
	}
	return r;
}

getstag(tag: int): ref Tag
{
	for(t := tags; t != nil; t = t.next){
		if(t.stag == tag){
			return t;
		}
	}
	return nil;
}

deltag(tag: ref Tag) {
	prev: ref Tag;
	for(t := tags; t != nil; t = t.next){
		if(t == tag)
			break;
		prev = t;
	}
	if(t == nil)
		return;
	if(prev == nil)
		tags = t.next;
	else
		prev.next = t.next;
	tag.client = nil;
	tag.seq = nil;
	tag.ctag = NOTAG;
	tag.next = freetags;
	tag.req = nil;
	freetags = tag;
}

newsfid(): int
{
	fid: int;
#	if(freefids != nil)
#		(fid, freefids) = (hd freefids, tl freefids);
#	else
		fid = maxfid++;
	return fid;
}

delsfid(sfid: int) {
log("\tfree sfid "+string sfid);
	for(fl := freefids; fl != nil; fl = tl fl){
		if(hd fl == sfid){
			panic("double free");
		}
	}
	freefids = sfid :: freefids;
}

# map fid from client to server.
Client.getfid(c: self ref Client, cfid: int): ref Fid
{
	slot := cfid % NFIDHASH;
	for(fl := c.fids[slot]; fl != nil; fl = tl fl)
		if((hd fl).cfid == cfid)
			return hd fl;
log("no fid found for " + string cfid);
	return nil;
}

panic(s: string) {
	sys->fprint(sys->fildes(2), "panic: %s\n", s);
	log("panic: "+s);
	s[-1] = 0;
}

Client.newfid(c: self ref Client, cfid: int, seqstag: int): ref Fid
{
	slot := cfid % NFIDHASH;
	fid := ref Fid(cfid, newsfid(), 1, seqstag);
	for(fl := c.fids[slot]; fl != nil; fl = tl fl){
		if((hd fl).cfid == cfid){
			panic(sys->sprint("duplicate fid %d, old sfid %d", cfid, (hd fl).sfid));
		}
	}
	c.fids[slot] = fid :: c.fids[slot];
	return fid;
}

Client.delfid(c: self ref Client, fid: ref Fid) {
	delsfid(fid.sfid);
	if(c == nil){
		return;
	}
	slot := fid.cfid % NFIDHASH;
	p: list of ref Fid;
	for(q := c.fids[slot]; q != nil; q = tl q){
		if(hd q == fid){
			p = join(p, tl q);
			break;
		}
		p = hd q :: p;
	}
	c.fids[slot] = p;
}

join(x, y: list of ref Fid): list of ref Fid
{
	if(len x > len y)
		(x, y) = (y, x);
	for(; x != nil; x = tl x)
		y = hd x :: y;
	return y;
}
