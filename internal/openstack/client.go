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

	"github.com/cloudandheat/ch-k8s-lbaas/internal/config"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	tokens3 "github.com/gophercloud/gophercloud/openstack/identity/v3/tokens"

	netutil "k8s.io/apimachinery/pkg/util/net"
	certutil "k8s.io/client-go/util/cert"
)

type OpenStackClient struct {
	provider  *gophercloud.ProviderClient
	region    string
	projectID string
}

func NewProviderClient(cfg *config.AuthOpts) (*gophercloud.ProviderClient, error) {
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

func NewClient(cfg *config.AuthOpts) (*OpenStackClient, error) {
	provider, err := NewProviderClient(cfg)
	if err != nil {
		return nil, err
	}

	projectID := cfg.ProjectID
	if projectID == "" {
		projectID, err = getProjectID(provider)
		if err != nil {
			return nil, err
		}
	}

	return &OpenStackClient{
		provider:  provider,
		region:    cfg.Region,
		projectID: projectID,
	}, nil
}

func (client *OpenStackClient) NewNetworkV2() (*gophercloud.ServiceClient, error) {
	return openstack.NewNetworkV2(client.provider, gophercloud.EndpointOpts{
		Region: client.region,
	})
}

// Extract project ID from the provider client authentication result.
func getProjectID(provider *gophercloud.ProviderClient) (string, error) {
	authResult := provider.GetAuthResult()
	if authResult == nil {
		return "", fmt.Errorf("no AuthResult from provider client")
	}

	// We expect only identity v3 tokens
	token, ok := authResult.(tokens3.CreateResult)
	if !ok {
		return "", fmt.Errorf("unexpected AuthResult type %t", authResult)
	}

	project, err := token.ExtractProject()
	if err != nil {
		return "", err
	}
	return project.ID, nil
}
