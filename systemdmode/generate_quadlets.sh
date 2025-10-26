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
echo "Note: This script generates quadlet files. Directories will be created by deploy.sh on nodes."

# Output directory for quadlet files
OUTPUT_DIR="${OUTPUT_DIR:-$(pwd)/quadlets}"
mkdir -p "$OUTPUT_DIR"

# Generate router pod quadlet
cat > "$OUTPUT_DIR/routerpod.pod" <<EOF
[Unit]
Description=Router Pod
Wants=network-online.target
After=network-online.target

[Pod]
PodmanArgs=--share=+pid

[Service]
Restart=always

[Install]
WantedBy=default.target
EOF

# Generate controller pod quadlet
cat > "$OUTPUT_DIR/controllerpod.pod" <<EOF
[Unit]
Description=Controller Pod
Wants=network-online.target
After=network-online.target

[Pod]

[Service]
Restart=always

[Install]
WantedBy=default.target
EOF

# Generate volumes
cat > "$OUTPUT_DIR/frr-sockets.volume" <<EOF
[Volume]
EOF

cat > "$OUTPUT_DIR/frrconfig.volume" <<EOF
[Volume]
EOF

cat > "$OUTPUT_DIR/reloader.volume" <<EOF
[Volume]
EOF

# Generate FRR container quadlet
cat > "$OUTPUT_DIR/frr.container" <<EOF
[Unit]
Description=FRR Routing Container
Wants=network-online.target
After=network-online.target

[Container]
EnvironmentFile=-/etc/containers/systemd/router.env
Image=\${FRR_IMAGE:-$FRR_IMAGE}
Pod=routerpod.pod
ContainerName=frr
PodmanArgs=--pidfile=/etc/perouter/frr/frr.pid
AddCapability=CAP_NET_BIND_SERVICE CAP_NET_ADMIN CAP_NET_RAW CAP_SYS_ADMIN
Environment=TINI_SUBREAPER=true
Volume=frr-sockets.volume:/var/run/frr:Z
Volume=frrconfig.volume:/etc/frr:Z
Exec=/bin/bash -c "for i in {1..10}; do command -v /etc/frr/daemons >/dev/null 2>&1 && break || sleep 5; done && chmod -R a+rw /var/run/frr && /sbin/tini -- /usr/lib/frr/docker-start & attempts=0; until [[ -f /etc/frr/frr.log || \\\$attempts -eq 60 ]]; do sleep 1; attempts=\\\$(( \\\$attempts + 1 )); done; tail -f /etc/frr/frr.log"

[Service]
Restart=always

[Install]
WantedBy=default.target
EOF

# Generate copier container quadlet
cat > "$OUTPUT_DIR/copier.container" <<EOF
[Unit]
Description=Config Copier Container
Wants=network-online.target
After=network-online.target

[Container]
EnvironmentFile=-/etc/containers/systemd/router.env
Image=\${ROUTER_IMAGE:-$ROUTER_IMAGE}
Pod=routerpod.pod
ContainerName=copier
Volume=frrconfig.volume:/etc/frr:Z
Volume=reloader.volume:/etc/frr_reloader:Z
Exec=/bin/sh -c "cp -rLf /tmp/frr/* /etc/frr && chmod -R a+rw /etc/frr && cp /reloader /etc/frr_reloader/reloader && chmod -R a+rw /etc/frr_reloader && sleep infinity"

[Service]
Restart=always

[Install]
WantedBy=default.target
EOF

# Generate reloader container quadlet
cat > "$OUTPUT_DIR/reloader.container" <<EOF
[Unit]
Description=FRR Reloader Container
Wants=network-online.target
After=network-online.target copier.service

[Container]
EnvironmentFile=-/etc/containers/systemd/router.env
Image=\${FRR_IMAGE:-$FRR_IMAGE}
Pod=routerpod.pod
ContainerName=reloader
Volume=frrconfig.volume:/etc/frr:Z
Volume=frr-sockets.volume:/var/run/frr:Z
Volume=/etc/perouter/frr/:/etc/perouter:Z
Volume=reloader.volume:/etc/frr_reloader:Z
Exec=/bin/bash -c "for i in {1..10}; do command -v /etc/frr_reloader >/dev/null 2>&1 && break || sleep 5; done && /etc/frr_reloader/reloader --frrconfig=/etc/perouter/frr.conf --loglevel=debug --unixsocket /etc/perouter/frr/frr.socket"

[Service]
Restart=always

[Install]
WantedBy=default.target
EOF

# Generate controller container quadlet
cat > "$OUTPUT_DIR/controller.container" <<EOF
[Unit]
Description=OpenPerouter Controller Container
Wants=network-online.target
After=network-online.target

[Container]
EnvironmentFile=-/etc/containers/systemd/router.env
Image=\${ROUTER_IMAGE:-$ROUTER_IMAGE}
Pod=controllerpod.pod
ContainerName=controller
Volume=/run/containerd/containerd.sock:/run/containerd/containerd.sock:rshared
Volume=/run/netns:/run/netns:rshared
Volume=/etc/perouter/frr:/etc/perouter/frr:rshared
Volume=/var/lib/hostambassador:/shared:rshared
Volume=${CONFIG_DIR}:/etc/openperouter:ro
Volume=/proc:/hostproc:ro
Environment=KUBECONFIG=/shared/kubeconfig
Network=host
SecurityLabelDisable=true
PodmanArgs=--pid=host
AddCapability=CAP_NET_BIND_SERVICE CAP_NET_ADMIN CAP_NET_RAW CAP_SYS_ADMIN
Exec=--loglevel debug --frrconfig /etc/perouter/frr/frr.conf --pid-path /etc/perouter/frr/frr.pid --frr-socket /etc/perouter/frr/frr.socket --mode host

[Service]
Restart=always

[Install]
WantedBy=default.target
EOF

# Generate example environment file
cat > "$OUTPUT_DIR/router.env.example" <<EOF
# Example environment file for OpenPerouter quadlets
# Copy this to /etc/containers/systemd/router.env and customize as needed
ROUTER_IMAGE=$ROUTER_IMAGE
FRR_IMAGE=$FRR_IMAGE
EOF

echo "Quadlet files generated in $OUTPUT_DIR"
echo "To use these files:"
echo "  1. Copy *.pod, *.container, and *.volume files to /etc/containers/systemd/"
echo "  2. (Optional) Copy router.env.example to /etc/containers/systemd/router.env and customize"
echo "  3. Run: systemctl daemon-reload"
echo "  4. Enable and start: systemctl enable --now routerpod.service controllerpod.service"
