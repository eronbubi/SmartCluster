package discover

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/libp2p/zeroconf/v2"

	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/testenv"
)

func TestToFoundPrefersPrivateIPv4(t *testing.T) {
	e := &zeroconf.ServiceEntry{Port: 8444, AddrIPv4: []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("192.168.1.20")}}
	e.Instance = "SmartCluster home"
	f, ok := toFound(e)
	if !ok || f.Address != "192.168.1.20:8444" || f.Instance != "SmartCluster home" {
		t.Errorf("got %+v %v", f, ok)
	}
	if _, ok := toFound(&zeroconf.ServiceEntry{Port: 8444, AddrIPv4: []net.IP{net.ParseIP("8.8.8.8")}}); ok {
		t.Error("public address accepted")
	}
}

// Multicast depends on the machine and network, so this round trip only runs
// on request: SC_TEST_MDNS=1 go test ./agent/internal/discover/
func TestAnnounceAndBrowse(t *testing.T) {
	testenv.RequireServers(t)
	if os.Getenv("SC_TEST_MDNS") == "" {
		t.Skip("set SC_TEST_MDNS=1 to test multicast on this machine")
	}
	srv, err := zeroconf.Register("SmartCluster mdns-test", enroll.ServiceType, "local.", 18444, []string{"v=1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()
	found, err := Browse(context.Background(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		if f.Instance == "SmartCluster mdns-test" {
			return
		}
	}
	t.Errorf("announcement not found, got %+v", found)
}
