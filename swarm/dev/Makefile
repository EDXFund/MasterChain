.PHONY: build cluster test

default: build

build:
	go build -o bin/swarm    github.com/EDXFund/MasterChain/cmd/swarm
	go build -o bin/geth     github.com/EDXFund/MasterChain/cmd/geth
	go build -o bin/bootnode github.com/EDXFund/MasterChain/cmd/bootnode

cluster: build
	scripts/boot-cluster.sh

test:
	go test -v github.com/EDXFund/MasterChain/swarm/...
