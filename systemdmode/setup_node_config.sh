#!/bin/bash
set -euo pipefail

log_info() {
    echo "[INFO] $*"
}

log_error() {
    echo "[ERROR] $*"
}

if [[ $# -lt 1 ]]; then
    log_error "Usage: $0 <kind-cluster-name> [config-dir]"
    log_error "Example: $0 pe-kind"
    log_error "Example: $0 pe-kind /path/to/config/files"
    exit 1
fi

CLUSTER_NAME="$1"
CONFIG_DIR="${2:-}"

if [[ -n "$CONFIG_DIR" ]]; then
    if [[ ! -d "$CONFIG_DIR" ]]; then
        log_error "Config directory does not exist: $CONFIG_DIR"
        exit 1
    fi
    log_info "Will copy files from $CONFIG_DIR to each node"
fi

NODES=$(kind get nodes --name "$CLUSTER_NAME" 2>/dev/null)
if [[ -z "$NODES" ]]; then
    log_error "No nodes found for kind cluster: $CLUSTER_NAME"
    log_error "Please check that the cluster exists with: kind get clusters"
    exit 1
fi

NODE_INDEX=0
for NODE in $NODES; do
    log_info "Creating configuration file for node $NODE with nodeIndex=$NODE_INDEX..."

    docker exec "$NODE" mkdir -p /var/lib/openperouter

    docker exec "$NODE" bash -c "cat > /var/lib/openperouter/node-config.yaml <<EOF
nodeIndex: $NODE_INDEX
logLevel: debug
EOF"

    log_info "  Configuration file created successfully"

    if [[ -n "$CONFIG_DIR" ]]; then
        log_info "  Copying files from $CONFIG_DIR to node $NODE..."
        for file in "$CONFIG_DIR"/*; do
            if [[ -f "$file" ]]; then
                filename=$(basename "$file")
                docker cp "$file" "$NODE:/var/lib/openperouter/$filename"
                log_info "    Copied: $filename"
            fi
        done
    fi

    NODE_INDEX=$((NODE_INDEX + 1))
done

log_info "All node configurations created"
