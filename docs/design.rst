Design
######

Load Balancer / Traffic Director implementation
===============================================

Options are:

- IPVS
- raw nftables

k8s/openstack/lb integration
============================

Components
----------

- k8s client:

  * watch resources for updates

- lb client: MUST run on each gateway node

  * write netfilter rules
  * write keepalived config and restart keepalived

- openstack client:

  * allocate/de-allocate ports
  * attach/detach ports
  * allocate/de-allocate floating IPs

There needs to be AT LEAST ONE k8s client, ONE lb client PER GATEWAY and AT LEAST ONE openstack client.

Data Flow
---------

k8s client --Endpoints--> lb client
k8s client --Public IP requests--> os client
os client --Public IPs--> k8s client
lb client --Errors--> k8s client

LB client interface
-------------------

.. function:: ensure_ha_listeners(local_ips: List[IP])

.. function:: ensure_services(services: List[Tuple[IP, Port, List[Protocol], List[Tuple[IP, Port]]]])

OS client interface
-------------------

.. function:: 

Edge Cases
==========

1. How to distinguish a LB IP added to a Service by a user vs. by the controller?

   -> we need to know what our local state is.
