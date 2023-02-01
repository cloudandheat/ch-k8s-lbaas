# Config Options

Options with a `-` as default value are mandatory.

## Agent

| Name          | Type                             | Default | Description                                  |
|---------------|----------------------------------|---------|----------------------------------------------|
| shared-secret | string                           | -       | Secret that is shared with the controller(s) |
| bind-address  | string                           | -       | Bind IP address                              |
| bind-port     | int                              | -       | Bind TCP port                                |
| keepalived    | [Keepalived](#agent--keepalived) | ...     | Keepalived configuration                     |
| nftables      | [Nftables](#agent--nftables)     | ...     | Nftables configuration                       |

### Agent: Keepalived

| Name                   | Type                                   | Default   | Description                                                     |
|------------------------|----------------------------------------|-----------|-----------------------------------------------------------------|
| enabled                | bool                                   | true      | Enable keepalived config update                                 |
| vrrp-password          | string                                 | "useless" | The VRRP password that is used, should be the same on all nodes |
| priority               | int                                    | 0         | The VRRP priority of the node                                   |
| virtual-router-id-base | int                                    | -         | Virtual Router ID base                                          |
| interface              | string                                 | -         | Network interface used for VRRP                                 |
| service                | [ServiceConfig](#agent--serviceconfig) | ...       | Keepalived service configuration                                |

### Agent: Nftables

| Name                  | Type                                   | Default         | Description                                                                                                                                                              |
|-----------------------|----------------------------------------|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| filter-table-name     | string                                 | "filter"        | Name of the nftables table for filtering rules                                                                                                                           |
| filter-table-type     | string                                 | "inet"          | Type of the nftables table for filtering rules                                                                                                                           |
| filter-forward-chain  | string                                 | "forward"       | Name of the nftables chain for filtering rules in the specified table                                                                                                    |
| nat-table-name        | string                                 | "nat"           | Name of the nftables table for NAT                                                                                                                                       |
| nat-prerouting-chain  | string                                 | "prerouting"    | Name of the nftables prerouting chain for NAT                                                                                                                            |
| nat-postrouting-chain | string                                 | "postrouting"   | Name of the nftables postrouting chain for NAT                                                                                                                           |
| policy-prefix         | string                                 | ""              | Prefix for nftables chains created for k8s network policies                                                                                                              |
| nft-command           | string list                            | ["sudo", "nft"] | Command to run `nft`; Required for live-reload                                                                                                                           |
| live-reload           | bool                                   | false           | If live-reload should be enabled; Causes lbaas-agent to load the last config on startup and include nft-commands to delete removed policy-chains in the generated config |
| enable-snat           | bool                                   | true            | If SNAT should be enabled; Can be false if the load-balancer is also default gateway for the k8s nodes                                                                   |
| fwmark-bits           | uint                                   | 1               | Mark that is used to mark load-balanced nftable/conntrack flows in the form: `mark 0x<FWMarkBits> and 0x<FWMarkMask>`                                                    |
| fwmark-mask           | uint                                   | 1               | See `FWMarkBits`                                                                                                                                                         |
| service               | [ServiceConfig](#agent--serviceconfig) | ...             | Nftables service configuration                                                                                                                                           |

### Agent: ServiceConfig

| Name           | Type        | Default                                                        | Description                                                                                                   |
|----------------|-------------|----------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| config-file    | string      | -                                                              | Path of the config file                                                                                       |
| reload-command | string list | ["sudo", "systemctl", "reload", "nftables" or "keepalived"]    | Command to reload the service                                                                                 |
| status-command | string list | ["sudo", "systemctl", "is-active", "nftables" or "keepalived"] | Command to get status of the service, used for healthcheck after reload. If empty, the healthcheck is skipped |
| start-command  | string list | ["sudo", "systemctl", "start", "nftables" or "keepalived"]     | Command to start the service                                                                                  |
| check-delay    | int         | 0                                                              | Delay (in seconds) between service reload and healthcheck                                                     |

## Controller

| Name          | Type                                | Default     | Description                                   |
|---------------|-------------------------------------|-------------|-----------------------------------------------|
| bind-address  | string                              | -           | Bind IP address                               |
| bind-port     | int                                 | 15203       | Bind TCP port                                 |
| port-manager  | string                              | "openstack" | Port manager to use ("openstack" or "static") |
| backend-layer | string                              | "NodePort"  | Backend layer to use                          |
| openstack     | [OpenStack](#controller--openstack) | ...         | OpenStack port manager configuration          |
| static        | [Static](#controller--static)       | ...         | Static port manager configuration             |
| agents        | [Agents](#controller--agents)       | ...         | Agents configuration                          |

### Controller: OpenStack

| Name    | Type                                       | Default | Description           |
|---------|--------------------------------------------|---------|-----------------------|
| auth    | [Auth](#controller--openstack--auth)       | ...     | Auth configuration    |
| network | [Network](#controller--openstack--network) | ...     | Network configuration |

### Controller: OpenStack: Auth

| Name                          | Type   | Default | Description  |
|-------------------------------|--------|---------|--------------|
| auth-url                      | string | -       | Keystone URL |
| user-id                       | string | ""      |              |
| username                      | string | ""      |              |
| password                      | string | ""      |              |
| project-id                    | string | ""      |              |
| project-name                  | string | ""      |              |
| trust-id                      | string | ""      |              |
| domain-id                     | string | ""      |              |
| domain-name                   | string | ""      |              |
| project-domain-id             | string | ""      |              |
| project-domain-name           | string | ""      |              |
| user-domain-id                | string | ""      |              |
| user-domain-name              | string | ""      |              |
| region                        | string | -       |              |
| ca-file                       | string | ""      |              |
| application-credential-id     | string | -       |              |
| application-credential-name   | string | ""      |              |
| application-credential-secret | string | -       |              |
| tls-insecure                  | bool   | false   |              |

### Controller: OpenStack: Network

| Name                   | Type   | Default | Description                     |
|------------------------|--------|---------|---------------------------------|
| use-floating-ips       | bool   | false   | If floating-IPs should be used  |
| floating-ip-network-id | string | ""      | UUID of the floating-IP network |
| subnet-id              | string | ""      | UUID of the internal network    |

### Controller: Static

| Name           | Type        | Default | Description                                              |
|----------------|-------------|---------|----------------------------------------------------------|
| ipv4-addresses | string list | []      | List of IPv4 address that can be used for load-balancing |

### Controller: Agents

| Name           | Type                           | Default | Description                            |
|----------------|--------------------------------|---------|----------------------------------------|
| shared-secret  | string                         | -       | Shared secret with the agents          |
| token-lifetime | int                            | 15      | Lifetime in seconds of the created JWT |
| agents         | [Agent](#ControllerAgent) list | -       | List of agents                         |

### Controller: Agents: Agent

| Name | Type   | Default | Description                    |
|------|--------|---------|--------------------------------|
| url  | string | -       | URL to the agent HTTP endpoint |
