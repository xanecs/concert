package main

import (
	"log"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns"
	"github.com/go-acme/lego/v4/registration"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
)

type ConcertConfig struct {
	ConsulAddress     string `yaml:"consulAddress"`
	AccountStoreKey   string `yaml:"accountStoreKey"`
	AccountEmail      string `yaml:"accountEmail"`
	CADir             string `yaml:"CADir"`
	DNS01ProviderName string `yaml:"DNS01ProviderName"`
	CertStoreKey      string `yaml:"certStoreKey"`
	RenewalTTL        string `yaml:"renewalTTL"`
	ReconcileInterval string `yaml:"reconcileInterval"`
}

type Concert struct {
	accountStore      *AccountStore
	account           *Account
	acmeClient        *lego.Client
	consulWatch       *watch.Plan
	consulAddress     string
	certStore         *CertStore
	renewalTTL        time.Duration
	reconcileInterval time.Duration
	consulCatalog     *api.Catalog
}

func NewConcert(config *ConcertConfig) (*Concert, error) {
	consulConfig := api.DefaultConfig()
	consulConfig.Address = config.ConsulAddress
	consul, err := api.NewClient(consulConfig)
	if err != nil {
		log.Println("Error connecting to consul")
		return nil, err
	}
	kv := consul.KV()

	certStore := NewCertStore(kv, config.CertStoreKey)

	accountStore := NewAccountStore(kv, config.AccountStoreKey)
	account, err := accountStore.LoadOrNew(config.AccountEmail)
	if err != nil {
		log.Println("Error getting account")
		return nil, err
	}

	acmeConfig := lego.NewConfig(account)

	acmeConfig.CADirURL = config.CADir
	acmeConfig.Certificate.KeyType = certcrypto.RSA2048

	acmeClient, err := lego.NewClient(acmeConfig)
	if err != nil {
		log.Println("Error creating ACME client")
		return nil, err
	}
	dnsProvider, err := dns.NewDNSChallengeProviderByName(config.DNS01ProviderName)
	if err != nil {
		log.Println("Error creating DNS01 provider")
		return nil, err
	}
	acmeClient.Challenge.SetDNS01Provider(dnsProvider)
	renewalTTL, err := time.ParseDuration(config.RenewalTTL)
	if err != nil {
		return nil, err
	}
	reconcileInterval, err := time.ParseDuration(config.ReconcileInterval)
	if err != nil {
		return nil, err
	}
	concert := &Concert{
		accountStore:      accountStore,
		account:           account,
		acmeClient:        acmeClient,
		consulAddress:     config.ConsulAddress,
		certStore:         certStore,
		renewalTTL:        renewalTTL,
		reconcileInterval: reconcileInterval,
		consulCatalog:     consul.Catalog(),
	}

	plan, err := watch.Parse(map[string]interface{}{"type": "services"})
	if err != nil {
		log.Fatalln(err)
	}
	plan.HybridHandler = concert.serviceHandler
	concert.consulWatch = plan

	return concert, nil
}

func (c *Concert) Register() error {
	reg, err := c.acmeClient.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return err
	}
	c.account.Registration = reg
	log.Printf("Account %s registered: %s\n", c.account.GetEmail(), reg.URI)
	return c.accountStore.Save(c.account)
}

func (c *Concert) Run() error {
	if c.account.Registration == nil {
		err := c.Register()
		if err != nil {
			log.Println("Error registering account")
			return err
		}
	} else {
		log.Printf("Account %s already registered\n", c.account.GetEmail())
	}

	go func() {
		for {
			time.Sleep(c.reconcileInterval)
			log.Println("Reconciling because of regular interval")
			services, _, err := c.consulCatalog.Services(&api.QueryOptions{})
			if err != nil {
				log.Printf("Error fetching services: %v\n", err)
				continue
			}
			domains := selectDomains(services)
			err = c.reconcile(domains)
			if err != nil {
				log.Printf("Error reconciling: %v\n", err)
			}
		}
	}()

	c.consulWatch.Run(c.consulAddress)
	return nil
}

func (c *Concert) serviceHandler(param watch.BlockingParamVal, data interface{}) {
	services, ok := data.(map[string][]string)
	if !ok {
		return
	}
	domains := selectDomains(services)
	log.Println("Reconciling because of consul update")
	err := c.reconcile(domains)
	if err != nil {
		log.Println(err)
	}
}

func (c *Concert) reconcile(domains []string) error {
	issued, err := c.certStore.List()
	if err != nil {
		return err
	}
	log.Println(issued)
	now := time.Now()
	for _, domain := range domains {
		expiry, ok := issued[domain]
		if !ok || expiry.Sub(now) < c.renewalTTL {
			err = c.issue(domain)
			if err != nil {
				log.Printf("Error issuing cert for %s: %v\n", domain, err)
			}
		}
	}
	return nil
}

func (c *Concert) issue(domain string) error {
	request := certificate.ObtainRequest{
		Domains: []string{domain},
		Bundle:  true,
	}
	certificates, err := c.acmeClient.Certificate.Obtain(request)
	if err != nil {
		return err
	}
	return c.certStore.Put(domain, certificates.PrivateKey, certificates.Certificate)
}

func selectDomains(services map[string][]string) []string {
	domains := make([]string, 0)
	for _, tags := range services {
		for _, tag := range tags {
			if strings.HasPrefix(tag, "concert-") {
				domains = append(domains, strings.TrimPrefix(tag, "concert-"))
			}
		}
	}
	return domains
}
