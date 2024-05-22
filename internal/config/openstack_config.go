package config

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/utils/openstack/clientconfig"
	"k8s.io/klog"
)

type AuthOpts struct {
	AuthURL           string `toml:"auth-url"`
	UserID            string `toml:"user-id"`
	Username          string `toml:"username"`
	Password          string `toml:"password"`
	ProjectID         string `toml:"project-id"`
	ProjectName       string `toml:"project-name"`
	TrustID           string `toml:"trust-id"`
	DomainID          string `toml:"domain-id"`
	DomainName        string `toml:"domain-name"`
	ProjectDomainID   string `toml:"project-domain-id"`
	ProjectDomainName string `toml:"project-domain-name"`
	UserDomainID      string `toml:"user-domain-id"`
	UserDomainName    string `toml:"user-domain-name"`
	Region            string `toml:"region"`
	CAFile            string `toml:"ca-file"`
	TLSInsecure       bool   `toml:"tls-insecure"`

	ApplicationCredentialID     string `toml:"application-credential-id"`
	ApplicationCredentialName   string `toml:"application-credential-name"`
	ApplicationCredentialSecret string `toml:"application-credential-secret"`
}

type NetworkingOpts struct {
	UseFloatingIPs      bool   `toml:"use-floating-ips"`
	FloatingIPNetworkID string `toml:"floating-ip-network-id"`
	SubnetID            string `toml:"subnet-id"`
}

type Config struct {
	Global     AuthOpts       `toml:"auth"`
	Networking NetworkingOpts `toml:"network"`
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
			ProjectID:                   cfg.ProjectID,
			ProjectName:                 cfg.ProjectName,
			DomainID:                    cfg.DomainID,
			DomainName:                  cfg.DomainName,
			ProjectDomainID:             cfg.ProjectDomainID,
			ProjectDomainName:           cfg.ProjectDomainName,
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
