# Use the latest kind node image as base
FROM kindest/node:v1.29.2

RUN apt update && apt install -y podman

# Configure podman for rootless operation
#RUN echo 'unqualified-search-registries = ["docker.io"]' > /etc/containers/registries.conf && \
#    mkdir -p /etc/containers && \
#    echo -e '[storage]\ndriver = "overlay"\n[storage.options]\nmount_program = "/usr/bin/fuse-overlayfs"' > /etc/containers/storage.conf

# Ensure podman works in the container environment
RUN mkdir -p /var/lib/containers
