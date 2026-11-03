// Package ephemeralkey provides process-local, disposable cryptographic fixtures.
// Import only from tests. No private key is stored in source or written to disk.
package ephemeralkey

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"sync"
)

var privatePEM = sync.OnceValue(func() string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("cannot generate disposable test key")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		panic("cannot marshal disposable test key")
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
})

func PEM() string { return privatePEM() }
