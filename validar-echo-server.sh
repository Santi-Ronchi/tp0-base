#!/bin/sh

TEST_MSG="[ECHO TEST] Hello"
SERVER='server'
PORT=12345


# Enviar mensaje y capturar la respuesta con timeout
RESPONSE=$(docker run --rm --network tp0_testing_net busybox:latest sh -c "echo '$TEST_MSG' | nc $SERVER $PORT")

if [ "$RESPONSE" = "$TEST_MSG" ]; then
    echo 'action: test_echo_server | result: success'
else
    echo 'action: test_echo_server | result: fail'
fi