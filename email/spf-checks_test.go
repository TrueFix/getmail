package email

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"blitiri.com.ar/go/spf"
)

type testResolver struct {
	txt     map[string][]string
	ips     map[string][]net.IPAddr
	mx      map[string][]*net.MX
	queries []string
}

func (r *testResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	r.queries = append(r.queries, name)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.txt[name], nil
}
func (r *testResolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return r.mx[name], nil
}
func (r *testResolver) LookupIPAddr(_ context.Context, name string) ([]net.IPAddr, error) {
	return r.ips[name], nil
}
func (r *testResolver) LookupAddr(context.Context, string) ([]string, error) { return nil, nil }

func TestSPFEvaluation(t *testing.T) {
	for _, tc := range []struct{ name, record, want string }{
		{"bare IPv4", "v=spf1 ip4:192.0.2.1 -all", "pass"},
		{"negative qualifier", "v=spf1 -ip4:192.0.2.1 +all", "fail"},
		{"softfail", "v=spf1 ~all", "softfail"},
		{"a", "v=spf1 a -all", "pass"},
		{"mx", "v=spf1 mx -all", "pass"},
		{"include", "v=spf1 include:allowed.test -all", "pass"},
		{"negative include", "v=spf1 -include:allowed.test +all", "fail"},
		{"failed include continues", "v=spf1 include:denied.test ip4:192.0.2.1 -all", "pass"},
		{"redirect", "v=spf1 redirect=allowed.test", "pass"},
		{"redirect is fallback", "v=spf1 redirect=allowed.test -all", "fail"},
		{"macro", "v=spf1 exists:%{i}.allowed.test -all", "pass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &testResolver{
				txt: map[string][]string{"envelope.test": {tc.record}, "allowed.test": {"v=spf1 ip4:192.0.2.1 -all"}, "denied.test": {"v=spf1 -all"}, "header.test": {"v=spf1 +all"}},
				ips: map[string][]net.IPAddr{"envelope.test": {{IP: net.ParseIP("192.0.2.1")}}, "mail.envelope.test": {{IP: net.ParseIP("192.0.2.1")}}, "192.0.2.1.allowed.test": {{IP: net.ParseIP("127.0.0.1")}}},
				mx:  map[string][]*net.MX{"envelope.test": {{Host: "mail.envelope.test"}}},
			}
			e := &Email{ClientIP: net.ParseIP("192.0.2.1"), EnvelopeFrom: EmailUser{Email: "sender@envelope.test"}, From: EmailUser{Email: "spoof@header.test"}, HELO: "helo.test"}
			passed, err := e.VerifySPFContext(context.Background(), spf.WithResolver(resolver))
			if err != nil || e.SPFResult != tc.want || passed != (tc.want == "pass") {
				t.Fatalf("SPF = %q (%t), err = %v", e.SPFResult, passed, err)
			}
			if len(resolver.queries) == 0 || resolver.queries[0] != "envelope.test" {
				t.Fatalf("wrong SPF identity: %v", resolver.queries)
			}
			queries := len(resolver.queries)
			if _, err := e.VerifySPFContext(context.Background(), spf.WithResolver(resolver)); err != nil {
				t.Fatal(err)
			}
			if len(resolver.queries) != queries {
				t.Fatal("SPF result was not cached")
			}
		})
	}
}

func TestSPFNullSenderUsesHELO(t *testing.T) {
	r := &testResolver{txt: map[string][]string{"helo.test": {"v=spf1 ip6:2001:db8::1 -all"}}}
	e := &Email{ClientIP: net.ParseIP("2001:db8::1"), HELO: "helo.test", From: EmailUser{Email: "visible@other.test"}}
	passed, err := e.VerifySPFContext(context.Background(), spf.WithResolver(r))
	if err != nil || !passed || len(r.queries) != 1 || r.queries[0] != "helo.test" {
		t.Fatalf("HELO check failed: %v, %v", passed, err)
	}
}

func TestSPFLookupLimit(t *testing.T) {
	r := &testResolver{txt: make(map[string][]string)}
	for i := 0; i < 12; i++ {
		r.txt[fmt.Sprintf("d%d.test", i)] = []string{fmt.Sprintf("v=spf1 include:d%d.test -all", i+1)}
	}
	e := &Email{ClientIP: net.ParseIP("192.0.2.1"), EnvelopeFrom: EmailUser{Email: "sender@d0.test"}}
	passed, err := e.VerifySPFContext(context.Background(), spf.WithResolver(r))
	if passed || e.SPFResult != "permerror" || err == nil || len(r.queries) > 11 {
		t.Fatalf("lookup limit not enforced: result=%s queries=%d err=%v", e.SPFResult, len(r.queries), err)
	}
}

func TestSPFContextDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := &Email{ClientIP: net.ParseIP("192.0.2.1"), EnvelopeFrom: EmailUser{Email: "sender@example.test"}}
	passed, err := e.VerifySPFContext(ctx, spf.WithResolver(&testResolver{}))
	if passed || e.SPFResult != "temperror" || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SPF check: %s, %v", e.SPFResult, err)
	}
	if _, err := e.VerifySPFContext(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached SPF error lost: %v", err)
	}
}
