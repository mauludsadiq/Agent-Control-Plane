package security

import (
"crypto/ecdsa"
"crypto/elliptic"
"crypto/rand"
"crypto/tls"
"crypto/x509"
"crypto/x509/pkix"
"encoding/pem"
"fmt"
"math/big"
"net"
"os"
"time"
)

// CertBundle holds a CA and issued leaf certificates.
type CertBundle struct {
CACert     *x509.Certificate
CAKey      *ecdsa.PrivateKey
CAPEMCert  []byte
CAPEMKey   []byte
}

// GenerateCA generates a self-signed CA certificate for dev/test use.
func GenerateCA(org string) (*CertBundle, error) {
key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
if err != nil {
return nil, fmt.Errorf("generate CA key: %w", err)
}

serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
tmpl := &x509.Certificate{
SerialNumber:          serial,
Subject:               pkix.Name{Organization: []string{org}, CommonName: org + " CA"},
NotBefore:             time.Now().Add(-time.Minute),
NotAfter:              time.Now().Add(87600 * time.Hour), // 10 years
IsCA:                  true,
KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
BasicConstraintsValid: true,
}

certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
if err != nil {
return nil, fmt.Errorf("create CA cert: %w", err)
}
cert, _ := x509.ParseCertificate(certDER)

keyDER, _ := x509.MarshalECPrivateKey(key)
return &CertBundle{
CACert:    cert,
CAKey:     key,
CAPEMCert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
CAPEMKey:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
}, nil
}

// IssueCert issues a leaf certificate signed by the CA.
// Use for both server and client certificates.
func (ca *CertBundle) IssueCert(commonName string, dnsNames []string, ips []net.IP) (certPEM, keyPEM []byte, err error) {
key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
if err != nil {
return nil, nil, fmt.Errorf("generate key: %w", err)
}

serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
tmpl := &x509.Certificate{
SerialNumber: serial,
Subject:      pkix.Name{CommonName: commonName},
NotBefore:    time.Now().Add(-time.Minute),
NotAfter:     time.Now().Add(8760 * time.Hour), // 1 year
KeyUsage:     x509.KeyUsageDigitalSignature,
ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
DNSNames:     dnsNames,
IPAddresses:  ips,
}

certDER, err := x509.CreateCertificate(rand.Reader, tmpl, ca.CACert, &key.PublicKey, ca.CAKey)
if err != nil {
return nil, nil, fmt.Errorf("create cert: %w", err)
}

keyDER, _ := x509.MarshalECPrivateKey(key)
certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
return certPEM, keyPEM, nil
}

// ServerTLSConfig returns a tls.Config for the ACP server with mTLS enabled.
// clientCAs is the CA pool that client certs must chain to.
func ServerTLSConfig(certPEM, keyPEM, caCertPEM []byte) (*tls.Config, error) {
cert, err := tls.X509KeyPair(certPEM, keyPEM)
if err != nil {
return nil, fmt.Errorf("parse server cert: %w", err)
}

pool := x509.NewCertPool()
if !pool.AppendCertsFromPEM(caCertPEM) {
return nil, fmt.Errorf("parse CA cert for client verification")
}

return &tls.Config{
Certificates: []tls.Certificate{cert},
ClientAuth:   tls.RequireAndVerifyClientCert,
ClientCAs:    pool,
MinVersion:   tls.VersionTLS13,
}, nil
}

// ClientTLSConfig returns a tls.Config for a client connecting with mTLS.
func ClientTLSConfig(certPEM, keyPEM, caCertPEM []byte) (*tls.Config, error) {
cert, err := tls.X509KeyPair(certPEM, keyPEM)
if err != nil {
return nil, fmt.Errorf("parse client cert: %w", err)
}

pool := x509.NewCertPool()
if !pool.AppendCertsFromPEM(caCertPEM) {
return nil, fmt.Errorf("parse CA cert for server verification")
}

return &tls.Config{
Certificates: []tls.Certificate{cert},
RootCAs:      pool,
MinVersion:   tls.VersionTLS13,
}, nil
}

// WriteCertFiles writes PEM-encoded cert and key to disk.
func WriteCertFiles(dir, name string, certPEM, keyPEM []byte) error {
if err := os.WriteFile(dir+"/"+name+".crt", certPEM, 0644); err != nil {
return err
}
return os.WriteFile(dir+"/"+name+".key", keyPEM, 0600)
}
