// A toy program that resolves host names and checks if hosts are up.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const usage = `
read list of hosts from file and report servers listening on ports 80 or 443.
usage: webcheck FILE
`

// hostInfo contains the data result of checking a host. It satisfies the
// Stringer interface.
type hostInfo struct {
	name  string
	addrs []string
	ports []string
	err   error
}

func (i *hostInfo) String() string {
	if i.err != nil {
		return fmt.Sprintf(
			"name:\t%v\nerror:\tcould not resolve: %v",
			i.name, i.err,
		)
	}

	var status, portsInfo string
	if len(i.ports) != 0 {
		status = "server is up"
		portsInfo = strings.Join(i.ports, ", ")
	} else {
		status = "server may be down"
		portsInfo = "no known HTTP(S) ports listening"
	}

	addrsInfo := strings.Join(i.addrs, ", ")
	return fmt.Sprintf(
		"name:\t%v\nips:\t%v\nstatus:\t%v\nports:\t%v\n",
		i.name, addrsInfo, status, portsInfo,
	)
}

func main() {
	flag.Parse()

	infile := flag.Arg(0)
	if infile == "" {
		log.Fatal("missing required file argument")
	}

	hosts, err := parseInfile(infile)
	if err != nil {
		log.Fatal(err)
	}
	if len(hosts) == 0 {
		log.Fatal("empty hosts file")
	}

	for r := range resolveAll(hosts) {
		fmt.Println(r)
	}
}

// parseInFile reads a file containing a list of host names and returns the
// results as an array.
//
// This expects host names to be newline-delimited; empty lines or lines
// beginning with "#" are ignored.
//
// If the named file cannot be read, this returns an error.
func parseInfile(file string) ([]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hosts []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		b := bytes.TrimSpace(s.Bytes())
		if len(b) != 0 && b[0] != '#' {
			hosts = append(hosts, string(b))
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return hosts, err
}

// Attempt to resolve each hostname and reach each host.
//
// This returns a channel from which to read results; when all checks finish,
// the channel is closed.
func resolveAll(hosts []string) <-chan *hostInfo {
	const timeout = time.Second * 5
	rc := make(chan *hostInfo, 1<<10)

	go func() {
		var wg sync.WaitGroup
		for _, h := range hosts {
			h := h
			wg.Add(1)
			go func() {
				rc <- resolveOne(h)
				wg.Done()
			}()
		}
		wg.Wait()
		close(rc)
	}()
	return rc
}

// Attempt to resolve a single host and return a *hostInfo as the result.
func resolveOne(host string) *hostInfo {
	hi := &hostInfo{name: host}
	if hi.addrs, hi.err = net.LookupHost(host); hi.err != nil {
		return hi
	}

	var (
		ports   = map[string]bool{"80": false, "443": false}
		timeout = time.Second * 3
	)
	for _, a := range hi.addrs {
		for p := range ports {
			conn, err := net.DialTimeout("tcp", a+":"+p, timeout)
			if err != nil {
				continue
			}
			conn.Close()
			ports[p] = true
		}
	}

	// De-dupe ports before adding them to hostInfo.
	//
	// This is an antipattern: hostInfo should really have methods to do
	// his itself so that this function doesn't muck around with internal
	// details. But this is just a slapdash golang demo for a friend, so
	// I won't tell if you won't.
	for p := range ports {
		if ports[p] {
			hi.ports = append(hi.ports, p)
		}
	}
	return hi
}
