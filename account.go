package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"log"

	"github.com/go-acme/lego/v4/registration"
	"github.com/hashicorp/consul/api"
)

type Account struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func NewAccount(email string) (*Account, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Account{Email: email, key: privateKey}, nil
}

func (a *Account) GetEmail() string {
	return a.Email
}

func (a *Account) GetRegistration() *registration.Resource {
	return a.Registration
}

func (a *Account) GetPrivateKey() crypto.PrivateKey {
	return a.key
}

func (a *Account) MarshalJSON() ([]byte, error) {
	var privateKey []byte
	var err error
	var keyType string
	switch k := a.key.(type) {
	case *ecdsa.PrivateKey:
		privateKey, err = x509.MarshalECPrivateKey(k)
		keyType = "ecdsa"
	case *rsa.PrivateKey:
		privateKey = x509.MarshalPKCS1PrivateKey(k)
		keyType = "rsa"
	default:
		err = errors.New("invalid private key")
	}
	if err != nil {
		return nil, err
	}
	type Alias Account
	return json.Marshal(&struct {
		Key     []byte
		KeyType string
		*Alias
	}{
		Key:     privateKey,
		KeyType: keyType,
		Alias:   (*Alias)(a),
	})
}

func (a *Account) UnmarshalJSON(data []byte) error {
	type Alias Account
	aux := &struct {
		Key     []byte
		KeyType string
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var err error
	switch aux.KeyType {
	case "ecdsa":
		a.key, err = x509.ParseECPrivateKey(aux.Key)
	case "rsa":
		a.key, err = x509.ParsePKCS1PrivateKey(aux.Key)
	}
	return err
}

type AccountStore struct {
	kv  *api.KV
	key string
}

func NewAccountStore(kv *api.KV, key string) *AccountStore {
	return &AccountStore{kv, key}
}

func (s *AccountStore) Save(account *Account) error {
	accountJson, err := json.Marshal(account)
	if err != nil {
		return err
	}
	_, err = s.kv.Put(&api.KVPair{Key: s.key, Value: accountJson}, &api.WriteOptions{})
	return err
}

func (s *AccountStore) Load() (account *Account, err error) {
	accountKV, _, err := s.kv.Get(s.key, &api.QueryOptions{})
	if err != nil {
		return
	}
	if accountKV == nil {
		account = nil
		return
	}
	err = json.Unmarshal(accountKV.Value, &account)
	return
}

func (s *AccountStore) LoadOrNew(email string) (*Account, error) {
	account, err := s.Load()
	if err != nil {
		log.Println("Error loading account")
		return nil, err
	}
	if account == nil || account.GetEmail() != email {
		account, err = NewAccount(email)
		if err != nil {
			log.Println("Error creating new account")
			return nil, err
		}
		err = s.Save(account)
		if err != nil {
			log.Println("Error saving account")
			return nil, err
		}
	}
	return account, nil
}
