#!/bin/bash

# Mensaje de prueba
MSG="hola123"

# Alpine trae netcat-openbsd, que sirve.
OUTPUT=$(echo "$MSG" | docker run --rm --network=tp0_testing_net alpine sh -c "apk add --no-cache netcat-openbsd >/dev/null 2>&1 && nc server 12345" 2>/dev/null)

if [ "$OUTPUT" = "$MSG" ]; then
  echo "action: test_echo_server | result: success"
else
  echo "action: test_echo_server | result: fail"
fi