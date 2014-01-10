package main

import (
	"bytes"
	"flag"
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2"
	"os"
	"regexp"
	"strings"
)

type cmd struct {
	name  string
	args  string
	f     func(cmd, *ec2.EC2, []string)
	flags *flag.FlagSet
}

var cmds []cmd

func main() {
	flag.Parse()
	if flag.Arg(0) == "" {
		errorf("no command")
		os.Exit(2)
	}
	auth, err := aws.EnvAuth()
	if err != nil {
		fatalf("envauth: %v", err)
	}
	conn := ec2.New(auth, aws.USEast)

	if flag.Arg(0) == "help" {
		for _, c := range cmds {
			c.printUsage()
		}
		return
	}

	for _, c := range cmds {
		if flag.Arg(0) == c.name {
			c.run(conn, flag.Args()[1:])
			return
		}
	}
	errorf("unknown command %q", flag.Arg(0))
	os.Exit(2)
}

func (c cmd) run(conn *ec2.EC2, args []string) {
	c.flags.Parse(args)
	c.f(c, conn, c.flags.Args())
}

func (c cmd) usage() {
	c.printUsage()
	os.Exit(2)
}

func (c cmd) printUsage() {
	errorf("%s %s", c.name, c.args)
	c.flags.PrintDefaults()
}

var groupsFlags struct {
	v   bool
	vv  bool
	ids bool
}

func init() {
	flags := flag.NewFlagSet("groups", flag.ExitOnError)
	flags.BoolVar(&groupsFlags.v, "v", false, "print name, id, owner and description of group")
	flags.BoolVar(&groupsFlags.vv, "vv", false, "print all attributes of group")
	flags.BoolVar(&groupsFlags.ids, "ids", false, "print group ids")
	cmds = append(cmds, cmd{
		"groups",
		"",
		groups,
		flags,
	})
}

func groups(c cmd, conn *ec2.EC2, _ []string) {
	resp, err := conn.SecurityGroups(nil, nil)
	check(err, "list groups")
	var b bytes.Buffer
	printf := func(f string, a ...interface{}) {
		fmt.Fprintf(&b, f, a...)
	}
	for _, g := range resp.Groups {
		switch {
		case groupsFlags.vv:
			printf("%s:%s %s %q\n", g.OwnerId, g.Name, g.Id, g.Description)
			for _, p := range g.IPPerms {
				printf("\t")
				printf("\t-proto %s -from %d -to %d", p.Protocol, p.FromPort, p.ToPort)
				for _, g := range p.SourceGroups {
					printf(" %s", g.Id)
				}
				for _, ip := range p.SourceIPs {
					printf(" %s", ip)
				}
				printf("\n")
			}
		case groupsFlags.v:
			printf("%s %s %q\n", g.Name, g.Id, g.Description)
		case groupsFlags.ids:
			printf("%s\n", g.Id)
		default:
			printf("%s\n", g.Name)
		}
	}
	os.Stdout.Write(b.Bytes())
}

var instancesFlags struct {
	addr  bool
	state bool
	all   bool
}

func init() {
	flags := flag.NewFlagSet("instances", flag.ExitOnError)
	flags.BoolVar(&instancesFlags.all, "a", false, "print terminated instances too")
	flags.BoolVar(&instancesFlags.addr, "addr", false, "print instance address")
	flags.BoolVar(&instancesFlags.state, "state", false, "print instance state")
	cmds = append(cmds, cmd{
		"instances",
		"",
		instances,
		flags,
	})
}

func instances(c cmd, conn *ec2.EC2, args []string) {
	resp, err := conn.Instances(nil, nil)
	if err != nil {
		fatalf("cannot get instances: %v", err)
	}
	var line []string
	for _, r := range resp.Reservations {
		for _, inst := range r.Instances {
			if !instancesFlags.all && inst.State.Name == "terminated" {
				continue
			}
			line = append(line[:0], inst.InstanceId)
			if instancesFlags.state {
				line = append(line, inst.State.Name)
			}
			if instancesFlags.addr {
				if inst.DNSName == "" {
					inst.DNSName = "none"
				}
				line = append(line, inst.DNSName)
			}
			fmt.Printf("%s\n", strings.Join(line, " "))
		}
	}
}

func init() {
	cmds = append(cmds, cmd{
		"terminate",
		"[instance-id ...]",
		terminate,
		flag.NewFlagSet("terminate", flag.ExitOnError),
	})
}

func terminate(c cmd, conn *ec2.EC2, args []string) {
	if len(args) == 0 {
		return
	}
	_, err := conn.TerminateInstances(args)
	if err != nil {
		fatalf("cannot terminate instances: %v", err)
	}
}

func init() {
	cmds = append(cmds, cmd{
		"delgroup",
		"[group ...]",
		delgroup,
		flag.NewFlagSet("delgroup", flag.ExitOnError),
	})
}

func delgroup(c cmd, conn *ec2.EC2, args []string) {
	hasError := false
	for _, g := range args {
		var ec2g ec2.SecurityGroup
		if secGroupPat.MatchString(g) {
			ec2g.Id = g
		} else {
			ec2g.Name = g
		}
		_, err := conn.DeleteSecurityGroup(ec2g)
		if err != nil {
			errorf("cannot delete %q: %v", g, err)
			hasError = true
		}
	}
	if hasError {
		os.Exit(1)
	}
}

func init() {
	flags := flag.NewFlagSet("auth", flag.ExitOnError)
	ipPermsFlags(flags)
	cmds = append(cmds, cmd{
		"auth",
		"group (sourcegroup|ipaddr)...",
		auth,
		flags,
	})
}

func auth(c cmd, conn *ec2.EC2, args []string) {
	if len(args) < 1 {
		c.usage()
	}
	_, err := conn.AuthorizeSecurityGroup(parseGroup(args[0]), ipPerms(args[1:]))
	check(err, "authorizeSecurityGroup")
}

func parseGroup(s string) ec2.SecurityGroup {
	var g ec2.SecurityGroup
	if secGroupPat.MatchString(s) {
		g.Id = s
	} else {
		g.Name = s
	}
	return g
}

func init() {
	flags := flag.NewFlagSet("revoke", flag.ExitOnError)
	ipPermsFlags(flags)
	cmds = append(cmds, cmd{
		"revoke",
		"group (sourcegroup|ipaddr)...",
		revoke,
		flags,
	})
}

func revoke(c cmd, conn *ec2.EC2, args []string) {
	if len(args) < 1 {
		c.usage()
	}
	_, err := conn.RevokeSecurityGroup(parseGroup(args[0]), ipPerms(args[1:]))
	check(err, "revokeSecurityGroup")
}

func init() {
	cmds = append(cmds, cmd{
		"mkgroup",
		"name description",
		mkgroup,
		flag.NewFlagSet("mkgroup", flag.ExitOnError),
	})
}

func mkgroup(c cmd, conn *ec2.EC2, args []string) {
	if len(args) != 2 {
		c.usage()
	}
	_, err := conn.CreateSecurityGroup(args[0], args[1])
	check(err, "create security group")
}

var ipperms struct {
	fromPort int
	toPort   int
	protocol string
}

func ipPermsFlags(flags *flag.FlagSet) {
	flags.IntVar(&ipperms.fromPort, "from", 0, "low end of port range")
	flags.IntVar(&ipperms.toPort, "to", 65535, "high end of port range")
	flags.StringVar(&ipperms.protocol, "proto", "tcp", "high end of port range")
}

var secGroupPat = regexp.MustCompile(`^sg-[a-z0-9]+$`)
var ipPat = regexp.MustCompile(`^[0-9']+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$`)
var groupNamePat = regexp.MustCompile(`^([0-9]+):(.*)$`)

func ipPerms(args []string) (perms []ec2.IPPerm) {
	if len(args) == 0 {
		fatalf("no security groups or ip addresses given")
	}
	var groups []ec2.UserSecurityGroup
	var ips []string
	for _, a := range args {
		switch {
		case ipPat.MatchString(a):
			ips = append(ips, a)
		case secGroupPat.MatchString(a):
			groups = append(groups, ec2.UserSecurityGroup{Id: a})
		case groupNamePat.MatchString(a):
			m := groupNamePat.FindStringSubmatch(a)
			groups = append(groups, ec2.UserSecurityGroup{
				OwnerId: m[1],
				Name:    m[2],
			})
		default:
			fatalf("%q is neither security group id nor ip address", a)
		}
	}
	return []ec2.IPPerm{{
		FromPort:     ipperms.fromPort,
		ToPort:       ipperms.toPort,
		Protocol:     ipperms.protocol,
		SourceGroups: groups,
		SourceIPs:    ips,
	}}
	return
}

func check(err error, e string, a ...interface{}) {
	if err == nil {
		return
	}
	fatalf("%s: %v", fmt.Sprintf(e, a...), err)
}

func errorf(f string, args ...interface{}) {
	if !strings.HasSuffix(f, "\n") {
		f += "\n"
	}
	f = "ec2: " + f
	fmt.Fprintf(os.Stderr, f, args...)
}

func fatalf(f string, args ...interface{}) {
	errorf(f, args...)
	os.Exit(2)
}
