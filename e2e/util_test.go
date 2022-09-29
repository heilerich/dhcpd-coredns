package e2e_test

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	"github.com/heilerich/dhcpd-coredns/parser"
	"github.com/stretchr/testify/assert"
	"inet.af/netaddr"
)

func templateLease(lease *parser.Lease) string {
	return fmt.Sprintf(`
lease %v {
	starts 5 2022/06/03 12:18:58;
	ends 5 2022/06/03 12:20:58;
	tstp 5 2022/06/03 12:20:58;
	tsfp 5 2022/06/03 12:20:58;
	atsfp 5 2022/06/03 12:20:58;
	cltt 5 2022/06/03 12:18:58;
	binding state free;
	hardware ethernet 00:50:56:af:a4:d5;
	uid "\377-\032\2413\000\002\000\000\253\021\261L\001}\301\202~\213";
	client-hostname "%v";
}
`, lease.Address.String(), lease.Name)
}

func writeFixturesFactory(t *testing.T, leaseFile *os.File) func(fixtures ...*parser.Lease) {
	return func(fixtures ...*parser.Lease) {
		for _, fixture := range fixtures {
			_, err := leaseFile.WriteString(templateLease(fixture))
			assert.NoError(t, err)
		}
		leaseFile.Sync()
	}
}

type fixture []*parser.Lease

func createFixtures(t *testing.T, args ...string) fixture {
	if len(args)%2 != 0 {
		t.Fatalf("invalid argument count for fixtures: %v", len(args))
	}

	count := len(args) / 2

	fixtures := make([]*parser.Lease, count)

	for i := range fixtures {
		fixtures[i] = &parser.Lease{Name: args[i*2], Address: netaddr.MustParseIP(args[i*2+1])}
	}

	return fixtures
}

func (f fixture) StringSlice() []string {
	s := make([]string, len(f))

	for i := range s {
		s[i] = fmt.Sprintf("%v %v", f[i].Name, f[i].Address.String())
	}

	return s
}

var letters = []rune("abcdefghijklmnopqrstuvwxyz1234567890")

func randomSuffix(n int) string {
	rand.Seed(time.Now().UnixNano())
	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

type TestResolver struct {
	resolver   func() *net.Resolver
	zoneSuffix string
	ctx        context.Context
	testingT   *testing.T
}

func testResolver(t *testing.T, ctx context.Context, suffix string) *TestResolver {
	return &TestResolver{
		resolver: func() *net.Resolver {
			return &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					return net.Dial("udp", "coredns:5354")
				},
			}
		},
		zoneSuffix: suffix,
		ctx:        ctx,
		testingT:   t,
	}
}

func (r *TestResolver) LookupName(name string) []netaddr.IP {
	res := []netaddr.IP{}

	ips, err := r.RetryLookup(r.ctx, "ip4", fmt.Sprintf("%v.%v.test", name, r.zoneSuffix))
	if err != nil {
		if !isNotFound(err) {
			r.testingT.Logf("failed to lookup address: %v", err)
			r.testingT.Fail()
		}
	}

	ips6, err := r.RetryLookup(r.ctx, "ip6", fmt.Sprintf("%v.%v.test", name, r.zoneSuffix))
	if err != nil {
		if !isNotFound(err) {
			r.testingT.Logf("failed to lookup v6 address: %v", err)
			r.testingT.Fail()
		}
	}

	ips = append(ips, ips6...)

	for _, ip := range ips {
		naddr, ok := netaddr.FromStdIP(ip)
		if !ok {
			r.testingT.Logf("failed to parse returned address: %v", ip)
			r.testingT.Fail()
		}
		res = append(res, naddr)
	}

	return res
}

func (r *TestResolver) RetryLookup(ctx context.Context, network, host string) ([]net.IP, error) {
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return []net.IP{}, err
		}

		ips, err := r.resolver().LookupIP(ctx, network, host)
		if err == nil || count > 10 || !isTimeout(err) {
			return ips, err
		}

		count++
		r.testingT.Logf("%v DNS timeout: %v", time.Now(), host)
		time.Sleep(10 * time.Millisecond)
	}
}

func isTimeout(err error) bool {
	dnsError, ok := err.(*net.DNSError)
	if ok && dnsError.Timeout() {
		return true
	}
	return false
}

func isNotFound(err error) bool {
	dnsError, ok := err.(*net.DNSError)
	if ok && dnsError.IsNotFound {
		return true
	}
	return false
}

func (r *TestResolver) CheckLeaseFixture(fixtures fixture) (fixture, bool) {
	res := fixture{}

	nameSet := make(map[string]struct{})

	for _, lease := range fixtures {
		nameSet[lease.Name] = struct{}{}
	}

	for name := range nameSet {
		ips := r.LookupName(name)
		for _, ip := range ips {
			res = append(res, &parser.Lease{Name: name, Address: ip})
		}
	}

	stringFix := fixtures.StringSlice()
	stringRes := res.StringSlice()

	ok := assert.ElementsMatch(&testing.T{}, stringFix, stringRes)

	return res, ok
}

func (r *TestResolver) AssertLeaseFixture(fixtures fixture) {
	var (
		res fixture
		ok  bool
	)

	for {
		res, ok = r.CheckLeaseFixture(fixtures)
		if ok {
			break
		}

		if r.ctx.Err() != nil {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	stringFix := fixtures.StringSlice()
	resFix := res.StringSlice()

	assert.ElementsMatch(r.testingT, stringFix, resFix)
}
