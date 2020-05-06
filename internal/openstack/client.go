/* Copyright 2020 CLOUD&HEAT Technologies GmbH
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package openstack

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/utils/openstack/clientconfig"

	netutil "k8s.io/apimachinery/pkg/util/net"
	certutil "k8s.io/client-go/util/cert"
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

type OpenStackClient struct {
	provider *gophercloud.ProviderClient
	region   string
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
	config.InsecureSkipVerify = cfg.TLSInsecure
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
