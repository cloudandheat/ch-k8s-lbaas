# Partial Reload

The partial-reload feature for nftables can be used if there is no `nftables` service or similar that can just be reloaded.

When the option is enabled, the behaviour of the agent changes as follows:

- The nftables config is reloaded on start of the agent, so that the last config is applied
- When generating the nftables config
    - `flush chain` statements are rendered for `FilterForwardChainName`, `NATPreroutingChainName` and `NATPostroutingChainName`
    - `delete chain` statements are rendered for all currently existing chains in the `FilterTableName` table starting with `PolicyPrefix`

These changes allow lbaas to run in an environment where it isn't possible to reload the complete nftables ruleset.
In this case, the changes can be applied with `nft -f`.

## Example Config

### VyOS 1.3

```toml
bind-address="0.0.0.0"
bind-port=15203

[keepalived]
enabled=false

[nftables]
filter-table-name="filter"
filter-table-type="ip"
filter-forward-chain="VYATTA_PRE_FW_IN_HOOK"
nat-table-name="nat"
nat-prerouting-chain="VYATTA_PRE_DNAT_HOOK"
nat-postrouting-chain="VYATTA_PRE_SNAT_HOOK"
partial-reload=true
policy-prefix="lbaas-"
nft-command=["sudo","nft"]
enable-snat=true

[nftables.service]
config-file="/var/lib/ch-k8s-lbaas-agent/nftables/lbaas.conf"
reload-command=["sudo", "nft", "-f", "/var/lib/ch-k8s-lbaas-agent/nftables/lbaas.conf"]
status-command=["true"]
start-command=["sudo", "nft", "-f", "/var/lib/ch-k8s-lbaas-agent/nftables/lbaas.conf"]
```

__Warning:__ With VyOS 1.4, the names of the nftables hook changed.
