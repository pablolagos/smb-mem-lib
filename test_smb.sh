#!/bin/bash

echo "==================================="
echo "Probando el servidor SMB en memoria"
echo "==================================="
echo ""

# Colores para output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

SERVER="localhost"
PORT="8445"
SHARE="MemoryShare"

echo -e "${BLUE}Servidor SMB corriendo en:${NC}"
echo "  - Dirección: $SERVER:$PORT"
echo "  - Share: $SHARE"
echo "  - Acceso: Guest (sin contraseña)"
echo ""

echo "==================================="
echo "OPCIONES PARA CONECTARTE:"
echo "==================================="
echo ""

echo -e "${GREEN}1. Desde Linux (mismo equipo):${NC}"
echo "   # Instalar cliente SMB si no lo tienes:"
echo "   sudo apt-get install smbclient"
echo ""
echo "   # Listar archivos:"
echo "   smbclient //localhost/MemoryShare -p 8445 -N -c 'ls'"
echo ""
echo "   # Conectarte interactivamente:"
echo "   smbclient //localhost/MemoryShare -p 8445 -N"
echo ""
echo "   # Leer un archivo:"
echo "   smbclient //localhost/MemoryShare -p 8445 -N -c 'get README.txt'"
echo ""

echo -e "${GREEN}2. Desde Windows (misma red):${NC}"
echo "   - Abre el Explorador de Windows"
echo "   - En la barra de direcciones escribe:"
echo "   \\\\192.168.68.107@8445\\MemoryShare"
echo ""
echo "   O desde CMD/PowerShell:"
echo "   net use Z: \\\\192.168.68.107@8445\\MemoryShare"
echo ""

echo -e "${GREEN}3. Desde macOS (misma red):${NC}"
echo "   - Finder > Go > Connect to Server (Cmd+K)"
echo "   - Escribe: smb://192.168.68.107:8445/MemoryShare"
echo ""
echo "   O desde Terminal:"
echo "   mount_smbfs //guest@192.168.68.107:8445/MemoryShare /Volumes/MemoryShare"
echo ""

echo "==================================="
echo "PRUEBA RÁPIDA (si tienes smbclient):"
echo "==================================="
echo ""

if command -v smbclient &> /dev/null; then
    echo -e "${GREEN}Listando archivos en el servidor...${NC}"
    echo ""
    smbclient //localhost/MemoryShare -p 8445 -N -c 'ls'
    echo ""
    echo -e "${GREEN}Leyendo README.txt...${NC}"
    echo ""
    smbclient //localhost/MemoryShare -p 8445 -N -c 'get README.txt -'
else
    echo "smbclient no está instalado."
    echo "Instálalo con: sudo apt-get install smbclient"
fi

echo ""
echo "==================================="
