#!/usr/bin/env python3
import sys

if len(sys.argv) != 3:
    print(f"Uso: {sys.argv[0]} <archivo_salida.yaml> <cantidad_clientes>")
    sys.exit(1)

archivo = sys.argv[1]
try:
    cantidad = int(sys.argv[2])
    if cantidad > 5:
        print("La cantidad máxima de clientes es 5")
        sys.exit(1)
except ValueError:
    print("La cantidad de clientes debe ser un número entero")
    sys.exit(1)

agencias_data = [
    {
        "nombre": "Santiago Lionel",
        "apellido": "Lorca", 
        "documento": "30904465",
        "nacimiento": "1999-03-17",
        "numero": "7574"
    },
    {
        "nombre": "María José",
        "apellido": "González",
        "documento": "32123456", 
        "nacimiento": "1995-07-22",
        "numero": "1234"
    },
    {
        "nombre": "Carlos Alberto",
        "apellido": "Pérez",
        "documento": "28765432",
        "nacimiento": "1992-11-30",
        "numero": "5678"
    },
    {
        "nombre": "Ana Laura",
        "apellido": "Rodríguez",
        "documento": "35098765",
        "nacimiento": "2000-02-14",
        "numero": "9012"
    },
    {
        "nombre": "Luis Fernando",
        "apellido": "Martínez",
        "documento": "29876543",
        "nacimiento": "1998-09-05",
        "numero": "3456"
    }
]

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
    f.write(f"      - TOTAL_AGENCIES={cantidad}\n")
    f.write("    networks:\n")
    f.write("      - testing_net\n")
    f.write("    restart: on-failure\n")
    f.write("    volumes:\n")
    f.write("      - ./server/config.ini:/config.ini:ro\n")
    f.write("      - ./bets.csv:/bets.csv:rw\n")

    # Clientes
    for i in range(1, cantidad + 1):
        #Usamos datos de las agencias ciclicamente
        agencia_idx = (i - 1) % 5
        agencia_data = agencias_data[agencia_idx]
        f.write(f"\n  client{i}:\n")
        f.write(f"    container_name: client{i}\n")
        f.write("    image: client:latest\n")
        f.write("    entrypoint: /client\n")
        f.write("    environment:\n")
        f.write(f"      - CLI_ID={i}\n")
        f.write(f"      - AGENCIA={i}\n")
        f.write(f"      - NOMBRE={agencia_data['nombre']}\n")
        f.write(f"      - APELLIDO={agencia_data['apellido']}\n")
        f.write(f"      - DOCUMENTO={agencia_data['documento']}\n")
        f.write(f"      - NACIMIENTO={agencia_data['nacimiento']}\n")
        f.write(f"      - NUMERO={agencia_data['numero']}\n")
        f.write("    networks:\n")
        f.write("      - testing_net\n")
        f.write("    depends_on:\n")
        f.write("      - server\n")
        f.write("    restart: on-failure\n")
        f.write("    volumes:\n")
        f.write("      - ./client/config.yaml:/config.yaml:ro\n")
        f.write("      - ./.data/:/data/:ro\n")

    # Redes
    f.write("\nnetworks:\n")
    f.write("  testing_net:\n")
    f.write("    ipam:\n")
    f.write("      driver: default\n")
    f.write("      config:\n")
    f.write("        - subnet: 172.25.125.0/24\n")