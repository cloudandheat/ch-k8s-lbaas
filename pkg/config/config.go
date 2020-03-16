package config

import (
	"io"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/cloudandheat/cah-loadbalancer/pkg/openstack"
)

type Agent struct {
	Address string `toml:"address"`
	Port    int32  `toml:"port"`
}

type Agents struct {
	SharedSecret string  `toml:"shared-secret"`
	Agents       []Agent `toml:"agent"`
}

type ControllerConfig struct {
	OpenStack openstack.Config `toml:"openstack"`
	Agents    Agents           `toml:"agents"`
}

func ReadControllerConfig(config io.Reader) (result ControllerConfig, err error) {
	_, err = toml.DecodeReader(config, &result)
	return result, err
}

func ReadControllerConfigFromFile(path string) (ControllerConfig, error) {
	fin, err := os.Open(path)
	if err != nil {
		return ControllerConfig{}, err
	}
	defer fin.Close()
	return ReadControllerConfig(fin)
}
