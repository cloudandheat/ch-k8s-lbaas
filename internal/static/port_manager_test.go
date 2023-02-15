package static

import (
	"github.com/stretchr/testify/assert"
	"net/netip"
	"testing"
)

func newStaticPortManagerFixture(t *testing.T) *StaticL3PortManager {
	addr1, err := netip.ParseAddr("203.0.113.113")
	assert.Nil(t, err)
	addr2, err := netip.ParseAddr("198.51.100.100")
	assert.Nil(t, err)

	cfg := Config{
		IPv4Addresses: []netip.Addr{addr1, addr2},
	}

	man, err := NewStaticL3PortManager(&cfg)
	assert.Nil(t, err)

	return man
}

func TestCheckPortExists(t *testing.T) {
	man := newStaticPortManagerFixture(t)

	exists, err := man.CheckPortExists("203.0.113.113")
	assert.Nil(t, err)
	assert.True(t, exists)

	exists, err = man.CheckPortExists("198.51.100.100")
	assert.Nil(t, err)
	assert.True(t, exists)

	exists, err = man.CheckPortExists("222.222.222.222")
	assert.Nil(t, err)
	assert.False(t, exists)
}

func TestProvisionPort(t *testing.T) {
	man := newStaticPortManagerFixture(t)

	_, err := man.ProvisionPort()
	assert.NotNil(t, err)
}

func TestCleanUnusedPorts(t *testing.T) {
	man := newStaticPortManagerFixture(t)

	err := man.CleanUnusedPorts([]string{})
	assert.Nil(t, err)
}

func TestGetAvailablePorts(t *testing.T) {
	man := newStaticPortManagerFixture(t)

	ports, err := man.GetAvailablePorts()
	assert.Nil(t, err)
	assert.Equal(t, []string{"203.0.113.113", "198.51.100.100"}, ports)
}

func TestGetExternalAddress(t *testing.T) {
	man := newStaticPortManagerFixture(t)

	addr, fip, err := man.GetExternalAddress("203.0.113.113")
	assert.Nil(t, err)
	assert.Equal(t, "203.0.113.113", addr)
	assert.Equal(t, "", fip)

	_, _, err = man.GetExternalAddress("222.222.222.222")
	assert.NotNil(t, err)
}

func TestGetInternalAddress(t *testing.T) {
	man := newStaticPortManagerFixture(t)

	addr, err := man.GetInternalAddress("198.51.100.100")
	assert.Nil(t, err)
	assert.Equal(t, "198.51.100.100", addr)

	_, err = man.GetInternalAddress("222.222.222.222")
	assert.NotNil(t, err)
}
