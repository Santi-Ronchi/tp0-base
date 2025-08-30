#!/usr/bin/env python3
import sys

if len(sys.argv) != 3:
    print(f"Uso: {sys.argv[0]} <archivo_salida.yaml> <cantidad_clientes>")
    sys.exit(1)

archivo = sys.argv[1]
try:
    cantidad = int(sys.argv[2])
except ValueError:
    print("La cantidad de clientes debe ser un número entero")
    sys.exit(1)

with open(archivo, "w") as f:
    # Cabecera
    f.write("name: tp0\n")
    f.write("services:\n")
    f.write("  server:\n")
    f.write("    container_name: server\n")
    f.write("    image: server:latest\n")
    f.write("    entrypoint: python3 /main.py\n")
    f.write("    environment:\n")
    f.write("      - PYTHONUNBUFFERED=1\n")
    f.write("      - LOGGING_LEVEL=DEBUG\n")
    f.write("    networks:\n")
    f.write("      - testing_net\n")
    f.write("    volumes:\n")
    f.write("      - ./server/config.ini:/config.ini:ro\n")

    # Clientes
    for i in range(1, cantidad + 1):
        f.write(f"\n  client{i}:\n")
        f.write(f"    container_name: client{i}\n")
        f.write("    image: client:latest\n")
        f.write("    entrypoint: /client\n")
        f.write("    environment:\n")
        f.write(f"      - CLI_ID={i}\n")
        f.write("      - CLI_LOG_LEVEL=DEBUG\n")
        f.write("    networks:\n")
        f.write("      - testing_net\n")
        f.write("    depends_on:\n")
        f.write("      - server\n")
        f.write("    restart: on-failure\n")
        f.write("    volumes:\n")
        f.write("      - ./client/config.yaml:/config.yaml:ro\n")

    # Redes
    f.write("\nnetworks:\n")
    f.write("  testing_net:\n")
    f.write("    ipam:\n")
    f.write("      driver: default\n")
    f.write("      config:\n")
    f.write("        - subnet: 172.25.125.0/24\n")