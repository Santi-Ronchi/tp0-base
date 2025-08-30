#!/bin/bash

if [ $# -ne 2 ]; then
  echo "Uso: $0 <archivo_salida.yaml> <cantidad_clientes>"
  exit 1
fi

ARCHIVO=$1
CANTIDAD=$2

python3 mi-generador.py "$ARCHIVO" "$CANTIDAD"