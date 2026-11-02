#!/usr/bin/env bash
# Sincroniza el CSV de alimentos del repo legacy (data/) al paquete embebido (app/data/).
# go:embed no puede leer directorios padre, por eso existe esta copia.
set -euo pipefail
cd "$(dirname "$0")/.."
cp ../data/alimentos.csv data/alimentos.csv
echo "app/data/alimentos.csv sincronizado ($(wc -l < data/alimentos.csv) líneas)"
