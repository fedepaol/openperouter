#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=== Setup extra networks ==="

bash "$SCRIPT_DIR/setup_extra_networks.sh"

echo "=== Deploy frrk8s ==="

bash "$SCRIPT_DIR/deploy_frrk8s.sh"

echo "=== Enable routing ==="

bash "$SCRIPT_DIR/enable_routing.sh"

echo "=== Deploy OpenPERouter ==="

bash "$SCRIPT_DIR/deploy_openperouter.sh"


echo "=== Setup CLAB ==="
bash "$SCRIPT_DIR/setup-clab.sh"    

echo "=== Deployment complete ==="