#!/usr/bin/env bash

make -C ../.. ch-k8s-lbaas-agent
mkdir -p generated
../../ch-k8s-lbaas-agent -logtostderr -v 5 -config $PWD/agent-config.toml&
agent_pid=$!

sleep 1

./request.py
result=$?

kill $!

if [ "$result" -eq 0 ]; then
    cat generated/nftables.conf
fi
