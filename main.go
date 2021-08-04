package main

import (
	"log"
	"time"
)

func main() {
	concert, err := NewConcert(&ConcertConfig{
		ConsulAddress:     "http://localhost:8500",
		AccountStoreKey:   "concert/account",
		AccountEmail:      "acme@example.com",
		CADir:             "https://acme-staging-v02.api.letsencrypt.org/directory",
		DNS01ProviderName: "gcloud",
		CertStoreKey:      "certstore",
		ReconcileInterval: 10 * time.Minute,
	})
	if err != nil {
		log.Fatalln(err)
	}
	err = concert.Run()
	if err != nil {
		log.Fatalln(err)
	}
}
