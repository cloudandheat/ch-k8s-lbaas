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

Segregation
-----------

Controller
^^^^^^^^^^

- k8s client and openstack client
- may run in cluster or on gateway nodes (in cluster is easier!)
- listens to service updates
- generates desired OpenStack configuration based on current configuration + diffs
- applies desired OpenStack configuration to current cloud
- forwards keepalived/nftables config requests to agent

Agent
^^^^^

- http endpoint for controller to access
- runs on each gateway
- configures nftables and keepalived

API:

.. function:: reconfigure(...)

    Atomically reconfigure keepalived and nftables.

    This function is strongly exception safe. Any errors are returned grouped
    to the endpoints.

Edge Cases
==========

1. How to distinguish a LB IP added to a Service by a user vs. by the controller?

   -> we need to know what our local state is.


Managed Objects
===============

- k8s Service: external IP field
- OpenStack Port: one per external IP
- OpenStack Floating IP: up to one per external IP

Ports belong to N services.
Floating IPs belong to exactly one port.
A port may have 0 or 1 floating IPs.

How to keep state?

Option 1: In-process + annotations on Services
----------------------------------------------

Required annotations:

- k8s Service -> OS L2 Port ID

Initial sync:

- New service will not have annotation -> easy to recognize
- Existing service: treat L2 Port assignment as authoritative, check for
  conflicts, resolve conflicts by treating service as new
- Removed service: will not be seen at all -> stale state will linger

    - However: only L2 Ports and Floating IPs can be stale (since we can
      reconstruct the L4 port state from the initial sync)
    - -> after initial sync, enumerate OS L2 Ports which are tagged for us and
      remove any which are not assigned to any service
    - -> afterwards, enumerate tagged floating IPs and remove any which isn’t
      assigned (do a consistency check with the advertised external IPs of the
      services)

- -> after initial sync: send state apply command to agents


Events
------

- AddFunc:

    - managed && can be managed: RemapService
    - managed && can not be managed:

        1. if has L2 port annotation: UnmapService

            - if interrupted: initial sync will clear the managed flag and
              the usual recovery of an interrupted UnmapService will clean up
              the rest.

        2. set to unmanaged

    - not managed && can be managed: set to managed

        (any further processing will be in UpdateFunc)

    - not managed && can not be managed: ignore

- UpdateFunc:

    - managed && can be managed && has L2 port annotation: RemapService
    - managed && can be managed && has no L2 port annotation: MapService
    - managed && can not be managed:

        1. if has L2 port annotation: UnmapService

            - if interrupted: initial sync will clear the managed flag and
              the usual recovery of an interrupted UnmapService will clean up
              the rest.

        2. set to unmanaged

    - not managed && can not be managed: ignore

- DeleteFunc:

    - if not managed: ignore
    - if managed && has L2 port annotation: UnmapService


Operations
----------

**Note:** All operations are *not* concurrency safe. This means that we (1)
need to ensure that only a single Controller is active at a time and (2) each
controller only has one worker. We can use the `leaderelection` tool for that.


MapService
^^^^^^^^^^

1. Check if the current L2 port can satisfy the requested port ranges.

    - If interrupted: initial sync will treat new ports of the Service as well
      as old L2 port mapping as authoritative. This may cause the service to
      hop to a different IP address if a conflict arises.

2. If not satisfiable or no current L2 port: Look for L2 Port with matching available L4 port range

    - If interrupted: service will be re-discovered on sync and will be
      re-mapped

3. If available port found: Assign L2 Port to Service via Annotation and set
   External IP

   - If interrupted: service will have annotation, initial sync will recover
     the state based on that.

4. If available port not found:

    1. CreateAndAssignPort
    2. set annotations on service

5. UpdateAgents


CreateAndAssignPort
^^^^^^^^^^^^^^^^^^^

1. Create OpenStack port

    - If interrupted: port will be cleaned up by CleanUnusedPorts.

2. Assign floating IP to port

    - If interrupted: port will be cleaned up by CleanUnusedPorts, floating IP
      will too.


UnmapService
^^^^^^^^^^^^

1. Remove annotation from service

    - If interrupted: no annotated service will be there, L4 mappings and L2
      ports will be cleaned up, config will be regenerated

2. UpdateAgents
3. CleanUnusedPorts


CleanUnusedPorts
^^^^^^^^^^^^^^^^

1. Enumerate OpenStack ports with tag
2. Compare OpenStack ports against internal state
3. Delete all ports which are not used

    - If interrupted: initial sync calls this and unused ports will be cleaned
      up appropriately.

4. Enumerate OpenStack floating IPs with tag
5. Remove any unassociated floating IP

    - If interrupted: initial sync calls this and unused floating IPs will be
      cleaned up appropriately


UpdateAgents
^^^^^^^^^^^^

1. Generate request from internal state
2. Send request to all Agents

    - If interrupted: initial sync calls this

3. Add error states/events to affected Service objects

    - If interrupted: initial sync calls this
