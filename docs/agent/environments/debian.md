# Debian

This page describes the requirements for running the LBaaS-agent on a Debian host.

## Requirements

- nftables and keepalived installed
- Network interface where load-balancer IP-addresses will be configured by keepalived
- L3-connection to the k8s-cluster
    - For example by establishing BGP peerings with the kubernetes nodes

### Prepare nftables config

The nftables config must be adjusted to create required tables/chains and include our custom nftables config file. 
An example `/etc/nftables.conf` could look like this:

**Warning:** This is only an example and might not be secure!

```bash
table inet filter {
    chain input {
        type filter hook input priority 0; policy accept;
    }

    chain forward {
        type filter hook forward priority 0; policy accept;
    }

    chain output {
        type filter hook output priority 0; policy accept;
    }
}

table ip nat {
    chain postrouting {
        type nat hook postrouting priority 100;
    }

    chain prerouting {
        type nat hook prerouting priority 100;
    }
}

include "/var/lib/ch-k8s-lbaas-agent/nftables/*.conf"
```

## Example config

As most of the default config values can be used in this case, the configuration file is very slim.

```yaml
shared-secret="verysecure"
bind-address="0.0.0.0"
bind-port=15203

[keepalived]
interface="ens3"
priority=100
virtual-router-id-base=10

[keepalived.service]
config-file="/var/lib/ch-k8s-lbaas-agent/keepalived/lbaas.conf"
check-delay=2

[nftables.service]
config-file="/var/lib/ch-k8s-lbaas-agent/nftables/lbaas.conf"
```
