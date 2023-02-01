# Kubernetes Client

THe k8s client in the controller is responsible for managing services and to receive events of changed cluster resources.

## Events

The k8s clients listens for the following changes in the cluster:

- Services (Add/Update/Delete)

--> Triggers mapping/unmapping of corresponding services
--> Triggers configuration update

- Nodes (Add/Update/Delete), if `NodePort` backend-layer is used
- Endpoints (Add/Update/Delete), if `Pod` backend-layer is used
- NetworkPolicies (Add/Update/Delete)

--> Triggers configuration update
