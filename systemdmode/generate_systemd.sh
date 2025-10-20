#!/bin/bash

set -x

ROUTER_IMAGE="${IMAGE:-quay.io/openperouter/router:main}"
FRR_IMAGE="${IMAGE:-quay.io/frrouting/frr:10.2.1}"

#mkdir /run/netns
#mkdir -p /etc/perouter/frr

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
	-c "cp -rLf /tmp/frr/* /etc/frr && \
	 cp /reloader /etc/frr_reloader/reloader"


podman pod create --name=controllerpod

# Needs to be there before
#mkdir -p /var/lib/hostambassador

podman create --pod=controllerpod --name=controller \
	-v=/run/containerd/containerd.sock:/run/containerd/containerd.sock:rshared \
	-v=/run/netns:/run/netns:rshared \
	-v=/etc/perouter/frr:/etc/perouter/frr:rshared \
	-v /var/lib/hostambassador:/shared:rshared \
	-e KUBECONFIG=/shared/kubeconfig \
	--network=host \
	--cap-add=CAP_NET_BIND_SERVICE,CAP_NET_ADMIN,CAP_NET_RAW,CAP_SYS_ADMIN \
	--pid=host \
	-t "$ROUTER_IMAGE" \
	--loglevel debug --frrconfig /etc/perouter/frr/frr.conf --pid-path /etc/perouter/frr/frr.pid --frr-socket /etc/perouter/frr/frr.socket \
	--mode host

# Generate systemd unit files for both pods
podman generate systemd --new --files --name routerpod
podman generate systemd --new --files --name controllerpod

# Clean up the temporary pods and containers
# The --new flag ensures systemd units will create/remove them on start/stop
podman pod rm -f routerpod controllerpod
