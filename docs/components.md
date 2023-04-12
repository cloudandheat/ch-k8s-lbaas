# Components

LbaaS consists of two parts: The agent and the controller.

## Controller

> Maximum of one active controller at a time

- [Kubernetes client](controller/k8s_client.md)
    - Watching for changes to services, nodes, endpoints, network policies
    - Mapping new services of type `LoadBalancer`
- May run in k8s cluster or on gateway nodes (in cluster is easier!)
- [Port Manager](controller/port_manager.md), maybe with OpenStack client
- Generates structured keepalived/nftables config requests and sends them to all configured agents

## Agent

> One agent on every gateway/load-balancer node

- [HTTP endpoint](agent/api.md) for controller
- Generates [nftables](agent/nftables.md) and [keepalived](agent/keepalived.md) config and applies the changes
