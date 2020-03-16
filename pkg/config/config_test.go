package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	controllerCfgBlob = `
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
address="127.0.0.1"
port=12345

[[agents.agent]]
address="127.0.0.2"
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

	assert.Equal(t, "127.0.0.1", agents.Agents[0].Address)
	assert.Equal(t, int32(12345), agents.Agents[0].Port)

	assert.Equal(t, "127.0.0.2", agents.Agents[1].Address)
	assert.Equal(t, int32(0), agents.Agents[1].Port)
}
