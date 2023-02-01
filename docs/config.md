# Config Options

Options with a `-` as default value are mandatory.

## Agent

| Name         | Type                      | Default | Description                                  |
|--------------|---------------------------|---------|----------------------------------------------|
| SharedSecret | string                    | -       | Secret that is shared with the controller(s) |
| BindAddress  | string                    | -       | Bind IP address                              |
| BindPort     | int                       | -       | Bind TCP port                                |
| Keppalived   | [Keepalived](#Keepalived) | ...     | ...                                          |
| Nftables     | [Nftables](#Nftables)     | ...     | ...                                          |

### Keepalived

| Name         | Type                            | Default   | Description                                                     |
|--------------|---------------------------------|-----------|-----------------------------------------------------------------|
| Enabled      | bool                            | true      | Enable keepalived config update                                 |
| VRRPPassword | string                          | "useless" | The VRRP password that is used, should be the same on all nodes |
| Priority     | int                             | 0         | The VRRP priority of the node                                   |
| VRIDBase     | int                             | -         | Virtual Router ID base                                          |
| Interface    | string                          | -         | Network interface used for VRRP                                 |
| Service      | [ServiceConfig](#ServiceConfig) | ...       | ...                                                             |

### Nftables

| Name                    | Type                            | Default         | Description                                                                                                                                                              |
|-------------------------|---------------------------------|-----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| FilterTableName         | string                          | "filter"        | Name of the nftables table for filtering rules                                                                                                                           |
| FilterTableType         | string                          | "inet"          | Type of the nftables table for filtering rules                                                                                                                           |
| FilterForwardChainName  | string                          | "forward"       | Name of the nftables chain for filtering rules in the specified table                                                                                                    |
| NATTableName            | string                          | "nat"           | Name of the nftables table for NAT                                                                                                                                       |
| NATPreroutingChainName  | string                          | "prerouting"    | Name of the nftables prerouting chain for NAT                                                                                                                            |
| NATPostroutingChainName | string                          | "postrouting"   | Name of the nftables postrouting chain for NAT                                                                                                                           |
| PolicyPrefix            | string                          | ""              | Prefix for nftables chains created for k8s network policies                                                                                                              |
| NftCommand              | string list                     | ["sudo", "nft"] | Command to run `nft`; Required for live-reload                                                                                                                           |
| LiveReload              | bool                            | false           | If live-reload should be enabled; Causes lbaas-agent to load the last config on startup and include nft-commands to delete removed policy-chains in the generated config |
| EnableSNAT              | bool                            | true            | If SNAT should be enabled; Can be false if the load-balancer is also default gateway for the k8s nodes                                                                   |
| FWMarkBits              | uint                            | 1               | Mark that is used to mark load-balanced nftable/conntrack flows in the form: `mark 0x<FWMarkBits> and 0x<FWMarkMask>`                                                    |
| FWMarkMask              | uint                            | 1               | See `FWMarkBits`                                                                                                                                                         |
| Service                 | [ServiceConfig](#ServiceConfig) | ...             | ...                                                                                                                                                                      |

### ServiceConfig

| Name           | Type        | Default                                                       | Description                                                                                                   |
|----------------|-------------|---------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------|
| ConfigFile     | string      | -                                                             | Path of the config file                                                                                       |
| ReloadCommand  | string list | ["sudo", "systemctl", "reload", "nftables" or "keepalived"]   | Command to reload the service                                                                                 |
| StatusCommand  | string list | ["sudo", "systemctl", "is-active", "nftables" or "keepalived"]| Command to get status of the service, used for healthcheck after reload. If empty, the healthcheck is skipped |
| StartCommand   | string list | ["sudo", "systemctl", "start", "nftables" or "keepalived"]    | Command to start the service                                                                                  |
| CheckDelay     | int         | 0                                                             | Delay (in seconds) between service reload and healthcheck                                                     |