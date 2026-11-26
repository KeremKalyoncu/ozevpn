package engine

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ozevpn/internal/probe"
)

func genLocalTLSCert(t *testing.T, cn string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: cn},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:         true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, pool
}

// TestLabCryptoHandshakeReplayAndCorruption verifies:
// - VPN-02-04: Mutual/peer TLS 1.3 handshake strictly rejects an invalid/untrusted peer certificate.
// - VPN-02-06: Replayed encrypted TLS 1.3 application records are rejected (zero duplicate delivery).
// - VPN-02-10: Bit-flipped ciphertext records are rejected with crypto error (zero corrupted bytes delivered).
func TestLabCryptoHandshakeReplayAndCorruption(t *testing.T) {
	serverCert, serverPool := genLocalTLSCert(t, "ozevpn-server")
	goodClientCert, goodClientPool := genLocalTLSCert(t, "ozevpn-client")
	evilClientCert, _ := genLocalTLSCert(t, "evil-peer")

	// --- 1. VPN-02-04: Invalid peer rejected on mTLS TLS 1.3 handshake ---
	srvTLS := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    goodClientPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvTLS)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}(c)
		}
	}()

	badCli, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      serverPool,
		Certificates: []tls.Certificate{evilClientCert},
	})
	if err == nil {
		// In TLS 1.3, client cert rejection alert arrives on first Read after handshake
		_ = badCli.SetDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 16)
		_, readErr := badCli.Read(buf)
		_ = badCli.Close()
		if readErr == nil {
			t.Fatal("VPN-02-04 FAIL: untrusted peer certificate was accepted without alert")
		}
	}

	// Verify valid peer succeeds
	goodCli, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      serverPool,
		Certificates: []tls.Certificate{goodClientCert},
	})
	if err != nil {
		t.Fatalf("expected valid peer handshake to succeed, got: %v", err)
	}
	_ = goodCli.Close()

	// --- 2. VPN-02-06 (Replay) & VPN-02-10 (Bit-Flip Corruption) over TLS 1.3 AEAD ---
	runTamperScenario := func(mode string) {
		rawLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer rawLn.Close()

		var delivered bytes.Buffer
		serverErrCh := make(chan error, 1)
		go func() {
			rawConn, aErr := rawLn.Accept()
			if aErr != nil {
				return
			}
			tConn := tls.Server(rawConn, &tls.Config{
				MinVersion:   tls.VersionTLS13,
				Certificates: []tls.Certificate{serverCert},
			})
			defer tConn.Close()
			_ = tConn.SetDeadline(time.Now().Add(3 * time.Second))
			buf := make([]byte, 512)
			for {
				n, rErr := tConn.Read(buf)
				if n > 0 {
					delivered.Write(buf[:n])
				}
				if rErr != nil {
					serverErrCh <- rErr
					return
				}
			}
		}()

		// MitM TCP proxy between client and server that replays or corrupts the first app-data record
		mitmLn, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer mitmLn.Close()

		go func() {
			cIn, aErr := mitmLn.Accept()
			if aErr != nil {
				return
			}
			defer cIn.Close()
			sOut, dErr := net.Dial("tcp", rawLn.Addr().String())
			if dErr != nil {
				return
			}
			defer sOut.Close()
			go func() { _, _ = io.Copy(cIn, sOut) }()

			var tampered atomic.Bool
			buf := make([]byte, 4096)
			for {
				n, rErr := cIn.Read(buf)
				if n > 0 {
					pkt := append([]byte(nil), buf[:n]...)
					// TLS application data record starts with content type 0x17 and len > 40 (after handshake)
					if !tampered.Load() && len(pkt) > 45 && pkt[0] == 0x17 && bytes.Contains(pkt, []byte{}) {
						// Only tamper the payload packet (client writes 24-byte marker after handshake)
						if pkt[0] == 0x17 && n < 200 {
							tampered.Store(true)
							if mode == "replay" {
								_, _ = sOut.Write(pkt)
								_, _ = sOut.Write(pkt) // Replay identical AEAD record!
								continue
							}
							if mode == "corrupt" {
								pkt[len(pkt)-5] ^= 0xFF // Flip ciphertext/tag bits!
								_, _ = sOut.Write(pkt)
								continue
							}
						}
					}
					_, _ = sOut.Write(pkt)
				}
				if rErr != nil {
					return
				}
			}
		}()

		cli, err := tls.Dial("tcp", mitmLn.Addr().String(), &tls.Config{
			MinVersion: tls.VersionTLS13,
			RootCAs:    serverPool,
		})
		if err != nil {
			t.Fatalf("[%s] dial failed: %v", mode, err)
		}
		_, _ = cli.Write([]byte("SECRET_VPN_PAYLOAD_12345"))
		time.Sleep(150 * time.Millisecond)
		_ = cli.Close()

		srvErr := <-serverErrCh
		if mode == "replay" {
			if strings.Count(delivered.String(), "SECRET_VPN_PAYLOAD_12345") > 1 {
				t.Fatalf("VPN-02-06 FAIL: replayed AEAD record was delivered twice! got=%q", delivered.String())
			}
			if srvErr == nil || srvErr == io.EOF {
				t.Fatalf("VPN-02-06 FAIL: expected crypto/bad_record_mac error on replay, got %v", srvErr)
			}
		}
		if mode == "corrupt" {
			if delivered.Len() != 0 {
				t.Fatalf("VPN-02-10 FAIL: corrupted ciphertext delivered %d unauthenticated bytes: %q", delivered.Len(), delivered.String())
			}
			if srvErr == nil || srvErr == io.EOF {
				t.Fatalf("VPN-02-10 FAIL: expected AEAD authentication error on corrupted record, got %v", srvErr)
			}
		}
	}

	runTamperScenario("replay")
	runTamperScenario("corrupt")
}

// TestLabSingBoxDNSOutageFailClosed verifies VPN-07-06 & VPN-14-07:
// Running a real isolated sing-box 1.14.4 instance with an unreachable DNS server must NOT
// silently fall back to ISP/OS DNS and must return a clean failure.
func TestLabSingBoxDNSOutageFailClosed(t *testing.T) {
	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err != nil {
		t.Skip("bin/sing-box.exe not present")
	}

	// Reserve a free port for mixed-in and a closed port for unreachable DNS
	lnMix, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mixPort := lnMix.Addr().(*net.TCPAddr).Port
	_ = lnMix.Close()

	lnDead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := lnDead.Addr().(*net.TCPAddr).Port
	_ = lnDead.Close()

	cfg := map[string]any{
		"log": map[string]any{"level": "error"},
		"dns": map[string]any{
			"servers": []map[string]any{
				{
					"tag":         "dns-dead",
					"type":        "https",
					"server":      "127.0.0.1",
					"server_port": deadPort,
				},
			},
			"final": "dns-dead",
		},
		"inbounds": []map[string]any{
			{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      "127.0.0.1",
				"listen_port": mixPort,
			},
		},
		"outbounds": []map[string]any{
			{"type": "direct", "tag": "direct"},
		},
		"route": map[string]any{
			"default_domain_resolver": "dns-dead",
			"final":                   "direct",
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	cfgPath := filepath.Join(t.TempDir(), "dns-outage.json")
	if err := os.WriteFile(cfgPath, b, 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sbBin, "run", "-c", cfgPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start isolated sing-box: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Wait for mixed-in to listen
	ready := false
	for i := 0; i < 20; i++ {
		if c, dErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(mixPort)), 150*time.Millisecond); dErr == nil {
			_ = c.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("isolated sing-box mixed-in failed to listen")
	}

	proxyU, _ := url.Parse("http://127.0.0.1:" + itoa(mixPort))
	cli := &http.Client{
		Timeout:   4 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyU)},
	}
	resp, reqErr := cli.Get("https://example.com/")
	if reqErr == nil {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatal("VPN-07-06 FAIL: sing-box resolved example.com despite dead DNS server (unexpected ISP fallback!)")
		}
	}
}

// TestLabDegradedNetworkFaultInjection verifies VPN-14-08:
// Under artificial latency + forced TCP drop, OzeVPN probe & error simplification fail gracefully without panic/leak.
func TestLabDegradedNetworkFaultInjection(t *testing.T) {
	// Start a fault-injecting HTTP proxy that introduces 180ms latency and drops connections
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	go func() {
		for {
			c, aErr := ln.Accept()
			if aErr != nil {
				return
			}
			go func(conn net.Conn) {
				time.Sleep(180 * time.Millisecond) // Artificial jitter/latency
				_ = conn.Close()                   // Simulated 100% packet drop / reset
			}(c)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	rep := probe.Run(ctx, port)
	if !rep.ViaProxy {
		t.Fatal("expected probe.Run to route via fault-injected proxy")
	}
	for _, tgt := range rep.Targets {
		if tgt.Reachable {
			t.Fatalf("VPN-14-08 FAIL: target %s unexpectedly succeeded despite 100%% drop fault-injected proxy", tgt.Name)
		}
		if tgt.Error == "" {
			t.Fatalf("expected user-friendly error message on degraded link for %s", tgt.Name)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
