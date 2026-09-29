package nodetls

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nullstone-modules/vault-cluster/internal/aws/s3"
)

type memObjects map[string][]byte

func (m memObjects) Put(_ context.Context, bucket, key string, body []byte) error {
	m[bucket+"/"+key] = append([]byte(nil), body...)
	return nil
}

func (m memObjects) PutIfAbsent(_ context.Context, bucket, key string, body []byte) (bool, error) {
	id := bucket + "/" + key
	if _, exists := m[id]; exists {
		return false, nil
	}
	m[id] = append([]byte(nil), body...)
	return true, nil
}

func (m memObjects) Get(_ context.Context, bucket, key string) ([]byte, error) {
	b, ok := m[bucket+"/"+key]
	if !ok {
		return nil, errors.New("missing")
	}
	return append([]byte(nil), b...), nil
}

func (m memObjects) List(_ context.Context, bucket, prefix string) ([]string, error) {
	return nil, nil
}

func TestEnsureClusterCAFirstWins(t *testing.T) {
	store := memObjects{}
	cert1, key1, err := EnsureClusterCA(store, "b", "vault-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	cert2, key2, err := EnsureClusterCA(store, "b", "vault-snapshots")
	if err != nil {
		t.Fatal(err)
	}
	if string(cert1) != string(cert2) || string(key1) != string(key2) {
		t.Fatal("second node must reuse the first CA")
	}
	if _, ok := store["b/"+s3.ClusterCAKey("vault-snapshots")]; !ok {
		t.Fatal("CA object missing")
	}
}

func TestEnsureClusterCARequiresBucket(t *testing.T) {
	if _, _, err := EnsureClusterCA(memObjects{}, "", "vault-snapshots"); err == nil {
		t.Fatal("expected missing bucket to fail")
	}
}

func TestIssueLeafVerifiesAgainstCA(t *testing.T) {
	caCert, caKey, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	ip := net.ParseIP("10.0.0.8")
	leaf, _, err := IssueLeaf(caCert, caKey, []string{"localhost", "vault.internal"}, []net.IP{ip, net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(leaf)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caCert) {
		t.Fatal("ca pem")
	}
	if _, err := cert.Verify(x509.VerifyOptions{DNSName: "vault.internal", Roots: pool}); err != nil {
		t.Fatal(err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{DNSName: "localhost", Roots: pool}); err != nil {
		t.Fatal(err)
	}
}

func TestProvisionWritesFilesWithoutCAKey(t *testing.T) {
	dir := t.TempDir()
	store := memObjects{}
	if err := Provision(store, "b", "vault-snapshots", dir, "10.0.0.8"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ca.crt", "tls.crt", "tls.key"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "ca.key")); err == nil {
		t.Fatal("CA key must not be written to disk")
	}
}

func TestProvisionRejectsBadIP(t *testing.T) {
	if err := Provision(memObjects{}, "b", "vault-snapshots", t.TempDir(), "not-an-ip"); err == nil || !strings.Contains(err.Error(), "VAULT_TLS_IP") {
		t.Fatalf("got %v", err)
	}
}

var _ s3.ObjectStore = memObjects{}
