# VyOS

This page describes the requirements for running the LBaaS-agent on a VyOS host.

## Requirements

- VyOS 1.3 (1.4 probably also works with some config changes)
- L3-connection to the k8s-cluster
    - For example by establishing BGP peerings with the kubernetes nodes (see below)
- At least one configured SNAT or DNAT rule via the VyOS configuration interface 
    - The created rule can also be disabled
    - (Required because VyOS does not call the required NAT hooks if there are no NAT rules)
- LBaaS-Controller with static port manager
    - Static IP-address(es) must be configured on the VyOS router

## Notes

- Automatic keepalived configuration is not directly supported on VyOS. The high-availability configuration should be
    configured independent of LBaaS.
 
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


## BGP Configuration

The LBaaS agent must be able to reach kubernetes-internal IP addresses like nodes, pods and services.

One way to achieve this is using BGP. An example BGP config for VyOS could be created with this command:

```
set protocols bgp <vyos-as> neighbor <k8s-node-ip> remote-as <k8s-as>
```

The configuration should be applied for all nodes (`<k8s-node-ip>`), so that a peering is established with every 
kubernetes node.

For this to work, there must be some BGP routing daemon running on all nodes in the cluster.
One option is to use [Calico](https://docs.tigera.io/calico) as CNI for kubernetes, which brings built-in BGP support
(via bird).
An example `BGPPeer` configuration for Calico could look like this:

```yaml
apiVersion: crd.projectcalico.org/v1
kind: BGPPeer
metadata:
  name: vyos
spec:
  asNumber: <vyos-as>
  keepOriginalNextHop: true
  peerIP: <vyos-ip>
```

If multiple LBaaS agents should be used, multiple `BGPPeer` objects must be created.

You can see if the configuration was successful by

1. Checking if the BGP peerings are listed in `show bgp summary established`
2. Checking if the routes are present in `ìp route list`
