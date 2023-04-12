# Backend Layers

The backend-layer configuration option for the controller specified, which target should be used in the load-balancing
process.

There are currently three available backend layers:

## NodePort (default)

When using `NodePort` as backend layer, lbaas will balance the traffic to all nodes on the node port(s) specified in the
k8s `LoadBalancer` service.

## ClusterIP

When using `ClusterIP` as backend layer, lbaas will forward the traffic to the cluster IP of the k8s `LoadBalancer` service.
This implies that there is only one endpoint and the actual load-balancing between pods is done by the k8s cluster.

## Pod

When using `Pod` as backend layer, lbaas will register all pod IP-addresses that belong to the k8s `LoadBalancer` service 
as endpoint for load-balancing. The k8s-internal load-balancer is not used.