//go:build darwin

package sandbox

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"ingen/sorna/internal/policy"
)

// This probe exercises the actual Seatbelt network filter using only local
// listeners. Seatbelt can enforce a localhost host
// token plus a pinned port, so both address families and other local host
// addresses at that port are measured below.
func TestCodexBrokerSeatbeltNetworkScope(t *testing.T) {
	ipv4, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer ipv4.Close()
	pinnedPort := ipv4.Addr().(*net.TCPAddr).Port

	ipv6, err := net.ListenTCP("tcp6", &net.TCPAddr{IP: net.ParseIP("::1"), Port: pinnedPort})
	if err != nil {
		t.Logf("cannot bind IPv6 loopback on IPv4 broker port %d: %v", pinnedPort, err)
	} else {
		defer ipv6.Close()
	}

	other, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	otherPort := other.Addr().(*net.TCPAddr).Port
	if otherPort == pinnedPort {
		t.Fatal("listener allocator returned duplicate ports")
	}

	sealed, err := brokerNetworkTestPolicy(pinnedPort)
	if err != nil {
		t.Fatal(err)
	}
	probe := func(address string, wantAllowed bool) {
		t.Helper()
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		prepared, err := Prepare([]string{"/usr/bin/nc", "-n", "-z", "-w", "2", host, port}, t.TempDir(), sealed)
		if err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, prepared.Command[0], prepared.Command[1:]...)
		output, runErr := command.CombinedOutput()
		if wantAllowed && runErr != nil {
			t.Fatalf("sandboxed connect to allowed local endpoint %s failed: %v; output=%s", address, runErr, output)
		}
		if !wantAllowed && runErr == nil {
			t.Fatalf("sandboxed connect to non-allowlisted endpoint %s succeeded", address)
		}
		if ctx.Err() != nil {
			t.Fatalf("sandboxed connect to %s timed out: %v; output=%s", address, ctx.Err(), output)
		}
	}

	t.Run("IPv4 pinned port is reachable", func(t *testing.T) {
		probe(net.JoinHostPort("127.0.0.1", strconv.Itoa(pinnedPort)), true)
	})
	t.Run("IPv4 other port is denied", func(t *testing.T) {
		probe(net.JoinHostPort("127.0.0.1", strconv.Itoa(otherPort)), false)
	})
	if ipv6 != nil {
		t.Run("IPv6 loopback same pinned port is also reachable", func(t *testing.T) {
			probe(net.JoinHostPort("::1", strconv.Itoa(pinnedPort)), true)
		})
	}
	if localAddress := listenOnAssignedNonLoopback(pinnedPort); localAddress != nil {
		defer localAddress.Close()
		address := localAddress.Addr().String()
		t.Run("assigned nonloopback local address same pinned port is reachable", func(t *testing.T) {
			probe(address, true)
		})
	} else {
		t.Log("no available nonloopback IPv4 interface address for local-address scope probe")
	}
}

func brokerNetworkTestPolicy(port int) (policy.Sealed, error) {
	document := testPolicy()
	document.Policy["network"] = map[string]any{
		"mode": "allowlist",
		"allow": []any{map[string]any{
			"host": "localhost", "ports": []any{int64(port)}, "direction": "outbound", "purpose": "local broker probe",
		}},
	}
	return policy.Seal(document)
}

func listenOnAssignedNonLoopback(port int) *net.TCPListener {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
				continue
			}
			listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: ip.To4(), Port: port})
			if err == nil {
				return listener
			}
		}
	}
	return nil
}

func TestSeatbeltRejectsLiteralNetworkHostSyntax(t *testing.T) {
	profile := "(version 1)\n(deny default)\n(allow network-outbound (remote tcp \"127.0.0.1:34567\"))\n"
	command := exec.Command("/usr/bin/sandbox-exec", "-p", profile, "/usr/bin/true")
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "host must be * or localhost in network address") {
		t.Fatalf("literal IPv4 Seatbelt profile = %v; output=%s, want unsupported host syntax", err, output)
	}
}

func TestPrepareRejectsLiteralIPNetworkHosts(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "192.0.2.4", "::1"} {
		t.Run(host, func(t *testing.T) {
			document := testPolicy()
			document.Policy["network"] = map[string]any{"mode": "allowlist", "allow": []any{map[string]any{
				"host": host, "ports": []any{int64(34567)}, "direction": "outbound", "purpose": "unsupported literal IP probe",
			}}}
			sealed, err := policy.Seal(document)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Prepare([]string{"/usr/bin/true"}, t.TempDir(), sealed)
			if err == nil || !strings.Contains(err.Error(), "cannot enforce literal IP") {
				t.Fatalf("Prepare error = %v; want clear literal IP rejection", err)
			}
		})
	}
}
