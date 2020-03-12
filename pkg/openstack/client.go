package openstack

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/utils/openstack/clientconfig"

	gcfg "gopkg.in/gcfg.v1"
	netutil "k8s.io/apimachinery/pkg/util/net"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog"
)

type AuthOpts struct {
	AuthURL          string `gcfg:"auth-url" mapstructure:"auth-url" name:"os-authURL" dependsOn:"os-password|os-trustID"`
	UserID           string `gcfg:"user-id" mapstructure:"user-id" name:"os-userID" value:"optional" dependsOn:"os-password"`
	Username         string `name:"os-userName" value:"optional" dependsOn:"os-password"`
	Password         string `name:"os-password" value:"optional" dependsOn:"os-domainID|os-domainName,os-projectID|os-projectName,os-userID|os-userName"`
	TenantID         string `gcfg:"tenant-id" mapstructure:"project-id" name:"os-projectID" value:"optional" dependsOn:"os-password"`
	TenantName       string `gcfg:"tenant-name" mapstructure:"project-name" name:"os-projectName" value:"optional" dependsOn:"os-password"`
	TrustID          string `gcfg:"trust-id" mapstructure:"trust-id" name:"os-trustID" value:"optional"`
	DomainID         string `gcfg:"domain-id" mapstructure:"domain-id" name:"os-domainID" value:"optional" dependsOn:"os-password"`
	DomainName       string `gcfg:"domain-name" mapstructure:"domain-name" name:"os-domainName" value:"optional" dependsOn:"os-password"`
	TenantDomainID   string `gcfg:"tenant-domain-id" mapstructure:"project-domain-id" name:"os-projectDomainID" value:"optional"`
	TenantDomainName string `gcfg:"tenant-domain-name" mapstructure:"project-domain-name" name:"os-projectDomainName" value:"optional"`
	UserDomainID     string `gcfg:"user-domain-id" mapstructure:"user-domain-id" name:"os-userDomainID" value:"optional"`
	UserDomainName   string `gcfg:"user-domain-name" mapstructure:"user-domain-name" name:"os-userDomainName" value:"optional"`
	Region           string `name:"os-region"`
	CAFile           string `gcfg:"ca-file" mapstructure:"ca-file" name:"os-certAuthorityPath" value:"optional"`
	TLSInsecure      string `name:"os-TLSInsecure" value:"optional" matches:"^true|false$"`

	ApplicationCredentialID     string `gcfg:"application-credential-id" mapstructure:"application-credential-id" name:"os-applicationCredentialID" value:"optional"`
	ApplicationCredentialName   string `gcfg:"application-credential-name" mapstructure:"application-credential-name" name:"os-applicationCredentialName" value:"optional"`
	ApplicationCredentialSecret string `gcfg:"application-credential-secret" mapstructure:"application-credential-secret" name:"os-applicationCredentialSecret" value:"optional"`
}

type NetworkingOpts struct {
	UseFloatingIPs      bool   `gcfg:"use-floating-ips"`
	FloatingIPNetworkID string `gcfg:"floating-ip-network-id"`
	SubnetID            string `gcfg:"subnet-id"`
}

type Config struct {
	Global     AuthOpts
	Networking NetworkingOpts
}

type OpenStackClient struct {
	provider *gophercloud.ProviderClient
	region   string
}

func ReadConfig(config io.Reader) (Config, error) {
	if config == nil {
		return Config{}, fmt.Errorf("no OpenStack cloud provider config file given")
	}
	var cfg Config
	err := gcfg.FatalOnly(gcfg.ReadInto(&cfg, config))

	return cfg, err
}

func ReadConfigFromFile(path string) (Config, error) {
	fin, err := os.Open(path)
	defer fin.Close()
	if err != nil {
		return Config{}, err
	}
	return ReadConfig(fin)
}

func (cfg AuthOpts) ToAuthOptions() gophercloud.AuthOptions {
	opts := clientconfig.ClientOpts{
		// this is needed to disable the clientconfig.AuthOptions func env detection
		EnvPrefix: "_",
		AuthInfo: &clientconfig.AuthInfo{
			AuthURL:                     cfg.AuthURL,
			UserID:                      cfg.UserID,
			Username:                    cfg.Username,
			Password:                    cfg.Password,
			ProjectID:                   cfg.TenantID,
			ProjectName:                 cfg.TenantName,
			DomainID:                    cfg.DomainID,
			DomainName:                  cfg.DomainName,
			ProjectDomainID:             cfg.TenantDomainID,
			ProjectDomainName:           cfg.TenantDomainName,
			UserDomainID:                cfg.UserDomainID,
			UserDomainName:              cfg.UserDomainName,
			ApplicationCredentialID:     cfg.ApplicationCredentialID,
			ApplicationCredentialName:   cfg.ApplicationCredentialName,
			ApplicationCredentialSecret: cfg.ApplicationCredentialSecret,
		},
	}

	ao, err := clientconfig.AuthOptions(&opts)
	if err != nil {
		klog.V(1).Infof("Error parsing auth: %s", err)
		return gophercloud.AuthOptions{}
	}

	// Persistent service, so we need to be able to renew tokens.
	ao.AllowReauth = true

	return *ao
}

func NewProviderClient(cfg *AuthOpts) (*gophercloud.ProviderClient, error) {
	provider, err := openstack.NewClient(cfg.AuthURL)
	if err != nil {
		return nil, err
	}

	userAgent := gophercloud.UserAgent{}
	// FIXME: use a proper version here
	userAgent.Prepend(fmt.Sprintf("cah-loadbalancer-controller/0.0.0"))
	provider.UserAgent = userAgent

	var caPool *x509.CertPool
	if cfg.CAFile != "" {
		// read and parse CA certificate from file
		caPool, err = certutil.NewPool(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read and parse %s certificate: %s", cfg.CAFile, err)
		}
	}

	config := &tls.Config{}
	config.InsecureSkipVerify = cfg.TLSInsecure == "true"
	if caPool != nil {
		config.RootCAs = caPool
	}

	provider.HTTPClient.Transport = netutil.SetOldTransportDefaults(&http.Transport{TLSClientConfig: config})

	opts := cfg.ToAuthOptions()
	err = openstack.Authenticate(provider, opts)

	return provider, err
}

func NewClient(cfg *AuthOpts) (*OpenStackClient, error) {
	provider, err := NewProviderClient(cfg)
	if err != nil {
		return nil, err
	}

	return &OpenStackClient{
		provider: provider,
		region:   cfg.Region,
	}, nil
}

func (client *OpenStackClient) NewNetworkV2() (*gophercloud.ServiceClient, error) {
	return openstack.NewNetworkV2(client.provider, gophercloud.EndpointOpts{
		Region: client.region,
	})
}
