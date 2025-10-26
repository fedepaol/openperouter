#!/bin/bash

set -x
set -e  # Exit on error

ROUTER_IMAGE="${IMAGE:-quay.io/openperouter/router:main}"
FRR_IMAGE="${IMAGE:-quay.io/frrouting/frr:10.2.1}"

# Use /var/lib for config instead of /etc to avoid requiring root
# This directory will be created by deploy.sh on each node
CONFIG_DIR="${CONFIG_DIR:-/var/lib/openperouter}"

# Note: Directories will be created by deploy.sh on each node
# For local testing, you may need to create them manually:
#   sudo mkdir -p /run/netns /etc/perouter/frr /var/lib/hostambassador "${CONFIG_DIR}"
echo "Note: This script generates systemd files. Directories will be created by deploy.sh on nodes."

# Clean up any existing pods/containers first
echo "Cleaning up existing pods..."
podman pod rm -f routerpod controllerpod 2>/dev/null || true

podman pod create --share=+pid --name=routerpod 
podman create --pod=routerpod --name=frr \
	--pidfile=/etc/perouter/frr/frr.pid \
	--cap-add=CAP_NET_BIND_SERVICE,CAP_NET_ADMIN,CAP_NET_RAW,CAP_SYS_ADMIN \
	-e TINI_SUBREAPER=true \
	-v=frr-sockets:/var/run/frr:Z \
	-v=frrconfig:/etc/frr:Z \
	--entrypoint=/bin/bash \
	-t "$FRR_IMAGE" \
	-c "for i in {1..10}; do command -v /etc/frr/daemons >/dev/null 2>&1 && break || sleep 5; done && chmod -R a+rw /var/run/frr && /sbin/tini -- /usr/lib/frr/docker-start & attempts=0; until [[ -f /etc/frr/frr.log || \$attempts -eq 60 ]]; do sleep 1; attempts=\$(( \$attempts + 1 )); done; tail -f /etc/frr/frr.log"

podman create --pod=routerpod --name=reloader \
	-v=frrconfig:/etc/frr:Z \
	-v=frr-sockets:/var/run/frr:Z \
	-v=/etc/perouter/frr/:/etc/perouter:Z \
	-v=reloader:/etc/frr_reloader:Z \
	--entrypoint=/bin/bash \
	-t "$FRR_IMAGE" \
	-c "for i in {1..10}; do command -v /etc/frr_reloader >/dev/null 2>&1 && break || sleep 5; done && /etc/frr_reloader/reloader --frrconfig=/etc/perouter/frr.conf --loglevel=debug --unixsocket /etc/perouter/frr.socket"

podman create --pod=routerpod --name=copier \
	-v=frrconfig:/etc/frr:Z \
	-v=reloader:/etc/frr_reloader:Z \
	--entrypoint=/bin/sh \
	-t "$ROUTER_IMAGE" \
	-c "cp -rLf /tmp/frr/* /etc/frr && chmod -R a+rw /etc/frr && \
	 cp /reloader /etc/frr_reloader/reloader && chmod -R a+rw /etc/frr_reloader && sleep infinity"


podman pod create --name=controllerpod

podman create --pod=controllerpod --name=controller \
	-v=/run/containerd/containerd.sock:/run/containerd/containerd.sock:rshared \
	-v=/run/netns:/run/netns:rshared \
	-v=/etc/perouter/frr:/etc/perouter/frr:rshared \
	-v /var/lib/hostambassador:/shared:rshared \
	-v=/proc:/hostproc:ro \
	-e KUBECONFIG=/shared/kubeconfig \
	--privileged \
	--network=host \
	--cap-add=CAP_NET_BIND_SERVICE,CAP_NET_ADMIN,CAP_NET_RAW,CAP_SYS_ADMIN \
	--pid=host \
	-t "$ROUTER_IMAGE" \
	--loglevel debug --frrconfig /etc/perouter/frr/frr.conf --pid-path /etc/perouter/frr/frr.pid --frr-socket /etc/perouter/frr/frr.socket \
	--mode host

# Generate systemd unit files for both pods
podman generate systemd --new --files --name routerpod
podman generate systemd --new --files --name controllerpod

# Add the config directory mount to the generated controller service file
# This is done post-generation to avoid needing the directory to exist locally
echo "Adding config directory mount to container-controller.service..."
sed -i '/--name=controller/a\	-v='"${CONFIG_DIR}"':/etc/openperouter:ro \\' container-controller.service

# Clean up the temporary pods and containers
# The --new flag ensures systemd units will create/remove them on start/stop
podman pod rm -f routerpod controllerpod
