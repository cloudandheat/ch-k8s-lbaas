package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	controllerCfgBlob = `
bind-address = "127.0.0.1"
bind-port = 1234
backend-layer = "Pod"

[openstack.auth]
auth-url="http://foo"
user-id="012345"
username="some_username"
password="some_password"
project-id="123456"
project-name="some_project"
trust-id="some-trust-id"
domain-id="456789"
domain-name="some_domain"
project-domain-id="789456"
project-domain-name="some_project_domain"
user-domain-id="123789"
user-domain-name="some_user_domain"
region="the_region"
ca-file="path/to/file"
tls-insecure=true

application-credential-id="1548"
application-credential-name="credential_name"
application-credential-secret="sup3rs3cr3t"

[openstack.network]
use-floating-ips=true
floating-ip-network-id="123abc"
subnet-id="456def"

[agents]
shared-secret="base64-encoded-string"

[[agents.agent]]
url="http://127.0.0.1:8081"

[[agents.agent]]
url="http://127.0.0.2:8080"
`

	agentCfgBlob = `
shared-secret="some-base64-blob"
bind-address="192.168.23.42"
bind-port=31337

[keepalived]
priority=120
vrrp-password="bogus"

[keepalived.service]
config-file="/etc/keepalived/conf.d/foo.conf"

[nftables]
filter-table-name="filter"
filter-table-type="inet"
filter-forward-chain="forward"

nat-table-name="nat"
nat-prerouting-chain="prerouting"
nat-postrouting-chain="postrouting"

[nftables.service]
config-file="/etc/nft/nft.d/foo.conf"
`
)

func TestCanReadControllerConfig(t *testing.T) {
	r := strings.NewReader(controllerCfgBlob)
	cfg, err := ReadControllerConfig(r)
	assert.Nil(t, err)

	// check openstack options
	osa := &cfg.OpenStack.Global
	assert.Equal(t, "http://foo", osa.AuthURL)
	assert.Equal(t, "012345", osa.UserID)
	assert.Equal(t, "some_username", osa.Username)
	assert.Equal(t, "some_password", osa.Password)
	assert.Equal(t, "123456", osa.ProjectID)
	assert.Equal(t, "some_project", osa.ProjectName)
	assert.Equal(t, "some-trust-id", osa.TrustID)
	assert.Equal(t, "456789", osa.DomainID)
	assert.Equal(t, "some_domain", osa.DomainName)
	assert.Equal(t, "789456", osa.ProjectDomainID)
	assert.Equal(t, "some_project_domain", osa.ProjectDomainName)
	assert.Equal(t, "123789", osa.UserDomainID)
	assert.Equal(t, "some_user_domain", osa.UserDomainName)
	assert.Equal(t, "the_region", osa.Region)
	assert.Equal(t, "path/to/file", osa.CAFile)
	assert.True(t, osa.TLSInsecure)
	assert.Equal(t, "1548", osa.ApplicationCredentialID)
	assert.Equal(t, "credential_name", osa.ApplicationCredentialName)
	assert.Equal(t, "sup3rs3cr3t", osa.ApplicationCredentialSecret)

	osn := &cfg.OpenStack.Networking
	assert.True(t, osn.UseFloatingIPs)
	assert.Equal(t, "123abc", osn.FloatingIPNetworkID)
	assert.Equal(t, "456def", osn.SubnetID)

	// agent config
	agents := &cfg.Agents
	assert.Equal(t, "base64-encoded-string", agents.SharedSecret)
	assert.Equal(t, 2, len(agents.Agents))

	assert.Equal(t, "http://127.0.0.1:8081", agents.Agents[0].URL)

	assert.Equal(t, "http://127.0.0.2:8080", agents.Agents[1].URL)
}

func TestCanReadAgentConfig(t *testing.T) {
	r := strings.NewReader(agentCfgBlob)
	cfg, err := ReadAgentConfig(r)
	assert.Nil(t, err)

	assert.Equal(t, "some-base64-blob", cfg.SharedSecret)
	assert.Equal(t, "192.168.23.42", cfg.BindAddress)
	assert.Equal(t, int32(31337), cfg.BindPort)

	kc := &cfg.Keepalived
	assert.Equal(t, 120, kc.Priority)
	assert.Equal(t, "bogus", kc.VRRPPassword)
	assert.Equal(t, "/etc/keepalived/conf.d/foo.conf", kc.Service.ConfigFile)

	nftc := &cfg.Nftables
	assert.Equal(t, "/etc/nft/nft.d/foo.conf", nftc.Service.ConfigFile)
	assert.Equal(t, "filter", nftc.FilterTableName)
	assert.Equal(t, "inet", nftc.FilterTableType)
	assert.Equal(t, "forward", nftc.FilterForwardChainName)
	assert.Equal(t, "nat", nftc.NATTableName)
	assert.Equal(t, "postrouting", nftc.NATPostroutingChainName)
	assert.Equal(t, "prerouting", nftc.NATPreroutingChainName)
}

func TestFillAgentConfig(t *testing.T) {
	cfg := AgentConfig{}
	FillAgentConfig(&cfg)

	assert.Equal(t, "", cfg.SharedSecret)
	assert.Equal(t, "", cfg.BindAddress)
	assert.Equal(t, int32(0), cfg.BindPort)

	kc := &cfg.Keepalived
	assert.Equal(t, "", kc.Service.ConfigFile)
	assert.Equal(t, 0, kc.Priority)
	assert.Equal(t, "useless", kc.VRRPPassword)

	nftc := &cfg.Nftables
	assert.Equal(t, "", nftc.Service.ConfigFile)
	assert.Equal(t, "filter", nftc.FilterTableName)
	assert.Equal(t, "inet", nftc.FilterTableType)
	assert.Equal(t, "forward", nftc.FilterForwardChainName)
	assert.Equal(t, "nat", nftc.NATTableName)
	assert.Equal(t, "postrouting", nftc.NATPostroutingChainName)
	assert.Equal(t, "prerouting", nftc.NATPreroutingChainName)
}
