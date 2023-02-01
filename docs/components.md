# Components

Lbaas consists of two parts: The agent and the controller

## Controller

> Maximum of one active controller at a time

- K8s client
  - Watching for changes to services, nodes, endpoints, network policies
  - Mapping new services of type `LoadBalancer`
- May run in k8s cluster or on gateway nodes (in cluster is easier!)
- [Port Manager](controller/port_manager.md), maybe with OpenStack client
- Generates structured keepalived/nftables config requests and sends them to all configured agents

## Agent

> One agent on every gateway/load-balancer node

- HTTP endpoint for controller
- Generates nftables and keepalived config and applies the changes