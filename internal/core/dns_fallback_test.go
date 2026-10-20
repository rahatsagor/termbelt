package core

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestDNSSkipsSystemResolverThatDoesNotRespond(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on UDP: %v", err)
	}
	server := &dns.Server{PacketConn: conn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(req)
		if req.Question[0].Qtype == dns.TypeA {
			rr, _ := dns.NewRR(req.Question[0].Name + " 60 IN A 192.0.2.7")
			reply.Answer = append(reply.Answer, rr)
		}
		_ = w.WriteMsg(reply)
	})}
	go func() { _ = server.ActivateAndServe() }()
	defer server.Shutdown()

	// A bound socket that never replies stands in for a resolver that is gone.
	dead, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dead.Close()
	previous := systemResolverList
	systemResolverList = func() []string { return []string{dead.LocalAddr().String(), conn.LocalAddr().String()} }
	defer func() { systemResolverList = previous }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := NewEngine(Config{TimeoutSeconds: 20}).DNS(ctx, Request{Tool: "dns", Input: "example.test", Options: map[string]string{"type": "A"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Summary, conn.LocalAddr().String()) || len(res.Table.Rows) != 1 || res.Table.Rows[0][2] != "192.0.2.7" {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(strings.Join(res.Notes, " "), "did not respond") {
		t.Fatalf("notes do not mention the skipped resolver: %v", res.Notes)
	}
}

func TestToggleFavoriteLeavesCopiesIntact(t *testing.T) {
	original := Config{Favorites: []string{"a", "b", "c"}}
	copied := original
	copied.ToggleFavorite("a")
	if strings.Join(original.Favorites, ",") != "a,b,c" || strings.Join(copied.Favorites, ",") != "b,c" {
		t.Fatalf("original=%v copied=%v", original.Favorites, copied.Favorites)
	}
}
