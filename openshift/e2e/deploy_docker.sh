#!/usr/bin/env bash
# Install Docker Engine from Docker's official repository on a RHEL 9 or
# CentOS Stream 9 host.

set -euo pipefail

if [[ ! -r /etc/os-release ]]; then
    echo "cannot determine the operating system: /etc/os-release is missing" >&2
    exit 1
fi

# shellcheck disable=SC1091
source /etc/os-release
if [[ "${VERSION_ID%%.*}" != "9" ]] || [[ "${ID:-}" != "rhel" && "${ID:-}" != "centos" ]]; then
    echo "this script supports RHEL 9 and CentOS Stream 9 only (found ${PRETTY_NAME:-unknown})" >&2
    exit 1
fi

DOCKER_REPO_OS="$ID"

if (( EUID == 0 )); then
    SUDO=()
else
    command -v sudo >/dev/null 2>&1 || {
        echo "sudo is required when running as a non-root user" >&2
        exit 1
    }
    SUDO=(sudo)
fi

echo "Installing Docker Engine prerequisites"
"${SUDO[@]}" dnf -y install dnf-plugins-core

echo "Removing packages that conflict with Docker Engine"
"${SUDO[@]}" dnf -y remove \
    docker \
    docker-client \
    docker-client-latest \
    docker-common \
    docker-latest \
    docker-latest-logrotate \
    docker-logrotate \
    docker-engine \
    podman \
    runc

echo "Configuring Docker's ${DOCKER_REPO_OS} repository"
"${SUDO[@]}" dnf config-manager --add-repo \
    "https://download.docker.com/linux/${DOCKER_REPO_OS}/docker-ce.repo"

echo "Installing Docker Engine"
"${SUDO[@]}" dnf -y install \
    docker-ce \
    docker-ce-cli \
    containerd.io \
    docker-buildx-plugin \
    docker-compose-plugin

echo "Enabling and starting Docker Engine"
"${SUDO[@]}" systemctl enable --now docker
"${SUDO[@]}" docker info >/dev/null

echo "Docker Engine is installed and running"
