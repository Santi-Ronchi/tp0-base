#!/bin/sh

SERVER='server'
PORT=12345

TEST_MSG="[ECHO TEST] Hello"

# Enviar mensaje y capturar la respuesta con timeout
RESPONSE=$(echo "$TEST_MSG" | nc "$SERVER" "$PORT" -w 3 -q 1)

if [ "$RESPONSE" = "$TEST_MSG" ]; then
    echo 'action: test_echo_server | result: success'
else
    echo 'action: test_echo_server | result: fail'
fi