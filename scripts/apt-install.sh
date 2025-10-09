#!/bin/bash

main() {
  set -eux -o pipefail

  apt-get update
  local packages=("ca-certificates")
  if [ "$LLAMA_SERVER_VARIANT" = "generic" ] || [ "$LLAMA_SERVER_VARIANT" = "cpu" ]; then
    # Install Vulkan SDK
    local vulkan_version=1.4.321.1
    local arch
    arch=$(uname -m)
    wget -qO /tmp/vulkan-sdk.tar.xz https://sdk.lunarg.com/sdk/download/$vulkan_version/linux/vulkan-sdk-linux-"$arch"-$vulkan_version.tar.xz
    mkdir -p /opt/vulkan
    tar -xf /tmp/vulkan-sdk.tar.xz -C /tmp --strip-components=1
    mv /tmp/"$arch"/* /opt/vulkan/
    rm -rf /tmp/*
  fi

  apt-get install -y "${packages[@]}"
  rm -rf /var/lib/apt/lists/*
}

main "$@"

