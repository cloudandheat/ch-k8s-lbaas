# Keepalived

When keepalived is enabled in the configuration, it is used to make the load-balancer IP-addresses high-available
by using VRRP between the load-balancer nodes.

Each load-balancer IP-address is configured as virtual-address in keepalived.
The available load-balancer node with the highest keepalived-priority will be selected as master and will configure
the IP-addresses (as /32) on the given network interface
