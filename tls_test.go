package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func getTLSConfig(t *testing.T) *tls.Config {
	key, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Poxy Tester Inc."},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := &bytes.Buffer{}
	err = pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	require.NoError(t, err)

	keyBytes, err := x509.MarshalECPrivateKey(key)
	keyPEM := &bytes.Buffer{}
	err = pem.Encode(keyPEM, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	require.NoError(t, err)

	certificate, err := tls.X509KeyPair(certPEM.Bytes(), keyPEM.Bytes())
	require.NoError(t, err)

	return &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{certificate},
	}
}
