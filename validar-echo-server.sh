#!/bin/sh


DOCKER_NETWORK='tp0_testing_net'
TEST_IMAGE='echo-tester:latest'
TEST_SCRIPT='netcat-validar-echo-server.sh'


docker build -t "$TEST_IMAGE" ./netcat-validar-echo-server
docker run --rm --network="$DOCKER_NETWORK" "$TEST_IMAGE" sh -c "./$TEST_SCRIPT"
