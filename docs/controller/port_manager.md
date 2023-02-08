# Port Manager

The port manager is responsible for managing the load-balancer IP-addresses (also called L3-ports). 

## Tasks

The port manager can do the following tasks:

- Provisioning new L3-ports
- Deleting unused L3-ports
- Returning a list of existing L3-ports
- Returning the external IP-address of an L3-port
- Returning the internal IP-address of an L3-port (may be the same as the external address)
- Checking if a given L3-port exists

## Implementations

There are currently two implementations for the port manager.
The configuration can be done in the config file of the controller.

### Static

A simple implementation that just has a static list of IP-addresses that can be used for load-balancing.

- Provisioning new L3-ports or deleting unused ones is not possible.
- The ID of the L3-port is the load-balancer IP-address
- External and internal IP-addresses are the same (functions just return the given L3-port ID)

### OpenStack

A more complex implementation that can be used if the load-balancer gateways are running on OpenStack.

- Able to create new OpenStack ports with floating-IPs
- The ID of the L3-port is the OpenStack port ID (UUID)
- Unused L3-ports can be deleted using the cleanup function
- The external IP-address is the floating-IP, the internal IP-address is the internal address to which the floating-IP points to

