package nodetls

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/nullstone-modules/vault-cluster/internal/aws/s3"
)

const (
	caFileName   = "ca.crt"
	certFileName = "tls.crt"
	keyFileName  = "tls.key"
	validity     = 10 * 365 * 24 * time.Hour
)

type clusterCA struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func Provision(store s3.ObjectStore, bucket, prefix, dir, privateIP string) error {
	ip := net.ParseIP(privateIP)
	if ip == nil {
		return fmt.Errorf("VAULT_TLS_IP is not a valid IP: %q", privateIP)
	}
	if bucket == "" {
		return fmt.Errorf("SNAPSHOT_BUCKET is not set")
	}
	if dir == "" {
		return fmt.Errorf("VAULT_TLS_DIR is not set")
	}
	caCertPEM, caKeyPEM, err := EnsureClusterCA(store, bucket, prefix)
	if err != nil {
		return err
	}
	leafCert, leafKey, err := IssueLeaf(caCertPEM, caKeyPEM, []string{"localhost", "vault.internal"}, []net.IP{ip, net.ParseIP("127.0.0.1")})
	if err != nil {
		return err
	}
	return WriteNodeFiles(dir, caCertPEM, leafCert, leafKey)
}

func EnsureClusterCA(store s3.ObjectStore, bucket, prefix string) (certPEM, keyPEM []byte, err error) {
	if bucket == "" {
		return nil, nil, fmt.Errorf("SNAPSHOT_BUCKET is not set")
	}
	key := s3.ClusterCAKey(prefix)
	raw, err := store.Get(context.Background(), bucket, key)
	if err == nil {
		return parseClusterCA(raw)
	}
	certPEM, keyPEM, err = NewCA()
	if err != nil {
		return nil, nil, err
	}
	body, err := json.Marshal(clusterCA{Cert: string(certPEM), Key: string(keyPEM)})
	if err != nil {
		return nil, nil, err
	}
	won, err := store.PutIfAbsent(context.Background(), bucket, key, body)
	if err != nil {
		return nil, nil, err
	}
	if won {
		return certPEM, keyPEM, nil
	}
	raw, err = store.Get(context.Background(), bucket, key)
	if err != nil {
		return nil, nil, err
	}
	return parseClusterCA(raw)
}

func NewCA() (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "vault-cluster-ca"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return encodeCert(der), encodeKey(key), nil
}

func IssueLeaf(caCertPEM, caKeyPEM []byte, dnsNames []string, ips []net.IP) (certPEM, keyPEM []byte, err error) {
	caCert, caKey, err := parseCAKey(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "vault"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	return encodeCert(der), encodeKey(key), nil
}

func WriteNodeFiles(dir string, caCert, leafCert, leafKey []byte) error {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{caFileName, caCert, 0640},
		{certFileName, leafCert, 0640},
		{keyFileName, leafKey, 0600},
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, f.data, f.mode); err != nil {
			return err
		}
	}
	return nil
}

func parseClusterCA(raw []byte) (certPEM, keyPEM []byte, err error) {
	var stored clusterCA
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, nil, fmt.Errorf("cluster CA: %w", err)
	}
	if stored.Cert == "" || stored.Key == "" {
		return nil, nil, fmt.Errorf("cluster CA is missing cert or key")
	}
	if _, _, err := parseCAKey([]byte(stored.Cert), []byte(stored.Key)); err != nil {
		return nil, nil, err
	}
	return []byte(stored.Cert), []byte(stored.Key), nil
}

func parseCAKey(certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("cluster CA cert is not PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster CA cert: %w", err)
	}
	if !cert.IsCA {
		return nil, nil, fmt.Errorf("cluster CA cert is not a CA")
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("cluster CA key is not PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("cluster CA key: %w", err)
	}
	return cert, key, nil
}

func encodeCert(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func encodeKey(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}
