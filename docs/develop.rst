Develop
#######

Running ch-k8s-lbaas-controller locally
=======================================

During development you usually don't want to go through a full "Build Image / Push Image / Deploy Image" cycle - and you don't have to. As a shortcut you can run the controller locally on your workstation. You need

- a kubeconfig file which gives the controller the necessary permissions (you could just use your default `admin.conf`)
- the `controller-config.toml` which tells the controller how to interact with the OpenStack control plane. You can fetch it from the k8s control plane and place it in the same directory as the controller, e.g., via `kubectl get secret -n kube-system -o jsonpath='{.data.controller-config\.toml} | base64 -d > controller-config.toml'`

If you're using yaook/k8s, then you probably also have to adapt firewall rules on the (primary) gateway node. Add an entry such as `ip saddr 172.30.153.0/24 tcp dport $lbaas_agent_tcp_port accept;` to the file `/var/lib/ch-k8s-lbaas-agent/nftables/access.conf` and restart nftables via `sudo systemctl reload nftables`. Obviously that's an ephemeral change and you have to adapt the address range to your actual wireguard subnet.