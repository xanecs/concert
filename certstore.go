package main

import (
	"crypto/x509"
	"encoding/pem"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/consul/api"
)

type CertStore struct {
	kv  *api.KV
	key string
}

func NewCertStore(kv *api.KV, key string) *CertStore {
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}
	return &CertStore{kv, key}
}

func (c *CertStore) List() (map[string]time.Time, error) {
	kvs, _, err := c.kv.List(c.key, &api.QueryOptions{})
	if err != nil {
		return nil, err
	}
	certs := make(map[string]time.Time)
	for _, kv := range kvs {
		if strings.HasSuffix(kv.Key, "-cert.pem") {
			name := strings.TrimPrefix(strings.TrimSuffix(kv.Key, "-cert.pem"), c.key)
			block, _ := pem.Decode(kv.Value)
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				log.Println(err)
				continue
			}
			certs[name] = cert.NotAfter
		}
	}
	return certs, nil
}

func (c *CertStore) Put(domain string, key []byte, certbundle []byte) error {
	_, err := c.kv.Put(&api.KVPair{Key: c.key + domain + "-key.pem", Value: key}, &api.WriteOptions{})
	if err != nil {
		return err
	}
	_, err = c.kv.Put(&api.KVPair{Key: c.key + domain + "-cert.pem", Value: certbundle}, &api.WriteOptions{})
	if err != nil {
		return err
	}
	return nil
}
