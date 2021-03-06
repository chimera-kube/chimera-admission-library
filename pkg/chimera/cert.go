package chimera

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

func generateCert(ca []byte, host string, extraSANs []string, CAPrivateKey *rsa.PrivateKey) ([]byte, []byte, error) {
	caCertificate, err := x509.ParseCertificate(ca)
	if err != nil {
		return nil, nil, err
	}

	serialNumber, err := rand.Int(rand.Reader, (&big.Int{}).Exp(big.NewInt(2), big.NewInt(159), nil))
	if err != nil {
		return nil, nil, err
	}

	servingPrivateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, nil, err
	}

	sansHosts := []string{}
	sansIps := []net.IP{}

	switch host {
	case "localhost":
		sansIps = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
		sansHosts = []string{host}
	case "127.0.0.1":
		sansIps = []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}
		sansHosts = []string{"localhost"}
	default:
		hostIp := net.ParseIP(host)
		if hostIp != nil {
			sansIps = []net.IP{hostIp}
		} else {
			sansHosts = []string{host}
		}
	}

	for _, san := range extraSANs {
		sanIp := net.ParseIP(san)
		if sanIp == nil {
			sansHosts = append(sansHosts, san)
		} else {
			sansIps = append(sansIps, sanIp)
		}
	}

	newCertificate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:    "",
			Organization:  []string{""},
			Country:       []string{""},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		DNSNames:     sansHosts,
		IPAddresses:  sansIps,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	servingCert, err := x509.CreateCertificate(rand.Reader, &newCertificate, caCertificate, &servingPrivateKey.PublicKey, CAPrivateKey)
	if err != nil {
		return nil, nil, err
	}
	servingCertPEM, err := pemEncodeCertificate(servingCert)
	if err != nil {
		return nil, nil, err
	}
	servingPrivateKeyPKCS1 := x509.MarshalPKCS1PrivateKey(servingPrivateKey)
	servingPrivateKeyPEM, err := pemEncodePrivateKey(servingPrivateKeyPKCS1)
	if err != nil {
		return nil, nil, err
	}
	return servingCertPEM, servingPrivateKeyPEM, nil
}

func pemEncodeCertificate(certificate []byte) ([]byte, error) {
	caPEM := new(bytes.Buffer)
	err := pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certificate,
	})
	return caPEM.Bytes(), err
}

func pemEncodePrivateKey(privateKey []byte) ([]byte, error) {
	privateKeyPEM := new(bytes.Buffer)
	err := pem.Encode(privateKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKey,
	})
	return privateKeyPEM.Bytes(), err
}
