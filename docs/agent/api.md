# API

The agent offers an API to enable the controller to send an updated configuration.

The following endpoints are available:

1. `POST /v1/apply`
    - Content-Type: "application/jwt"
    - JWT encoded JSON content encoded with shared-secret
    - Python script for an example request can be found [here](../../hack/debug-agent/request.py) 
