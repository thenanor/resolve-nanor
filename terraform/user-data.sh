#!/bin/bash
set -euo pipefail

dnf install -y docker rsync git
systemctl enable --now docker
usermod -aG docker ec2-user

# docker compose v2 CLI plugin
mkdir -p /usr/local/lib/docker/cli-plugins
curl -SL "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64" \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

# buildx plugin (dnf's docker package ships an old buildx that's too
# old for current compose, which builds via `buildx bake`)
buildx_version="$(curl -sL -o /dev/null -w '%{url_effective}' https://github.com/docker/buildx/releases/latest | grep -oP '(?<=/tag/)v[\d.]+')"
curl -SL "https://github.com/docker/buildx/releases/download/${buildx_version}/buildx-${buildx_version}.linux-amd64" \
  -o /usr/local/lib/docker/cli-plugins/docker-buildx
chmod +x /usr/local/lib/docker/cli-plugins/docker-buildx

mkdir -p /home/ec2-user/resolve
chown -R ec2-user:ec2-user /home/ec2-user/resolve
