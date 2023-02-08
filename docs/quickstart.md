# Quickstart

Alternatively to the steps below, LBaaS can be deployed as part of [yaook-k8s](https://yaook.gitlab.io/k8s/quick-start.html ) on OpenStack.

## Requirements

- A load-balancer node (e.g. Debian or VyOS) with nftables and (optional) keepalived 
- A kubernetes cluster
- A network configuration on the load-balancer node that allows connections to th nodes/pods/service-addresses of the
  kubernetes cluster

## Setting up the agent
### Building the agent

1. Clone the repository
2. Run `m̀ake`
3. Copy `ch-k8s-lbaas-agent` binary to your load-balancer node

### Running the agent

1. Create an agent config file, and save it somewhere (e.g. `/etc/ch-k8s-lbaas-agent/config.toml`). An example for
    Debian can be found [here](agent/environments/debian.md).

2. Start the agent: `./ch-k8s-lbaas-agent --config <path-to-config>`
    - Starting the agent as `root` is not recommended for production environments. It's recommended to create a separate
        user and creating sudo rules that allow sudo usage for the required commands (e.g. `sudo nft`)
    - It's recommended to create a systemd service for the agent

## Setting up the controller (in cluster)

### Creating a configuration secret

Create a secret with the controller configuration by applying the following yaml file to the cluster:

(example config with static port manager)

```yaml
apiVersion: v1
stringData:
  controller-config.toml: |
    port-manager="static"
    backend-layer="Pod"

    [static]
    ipv4-addresses=["203.0.113.113"]

    [agents]
    shared-secret="verysecure"
    token-lifetime=60

    [[agents.agent]]
    url="http://192.0.2.2:15203"
kind: Secret
metadata:
  name: ch-k8s-lbaas-controller-config
  namespace: kube-system
type: Opaque
```

### Creating a deployment
The controller can be deployed by applying the following yaml file to the cluster:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ch-k8s-lbaas-controller
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ch-k8s-lbaas-controller
  strategy:
    rollingUpdate:
      maxSurge: 0
      maxUnavailable: 100%
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: ch-k8s-lbaas-controller
    spec:
      containers:
      - args:
        - --config
        - /config/controller-config.toml
        image: ghcr.io/cloudandheat/ch-k8s-lbaas/controller:0.5.0
        name: controller
        ports:
        - containerPort: 15203
          name: api
          protocol: TCP
        volumeMounts:
        - mountPath: /config
          name: config
          readOnly: true
      nodeSelector:
        node-role.kubernetes.io/master: ""
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
      - effect: NoSchedule
        key: node-role.kubernetes.io/control-plane
      volumes:
      - name: config
        secret:
          defaultMode: 420
          secretName: ch-k8s-lbaas-controller-config

```
