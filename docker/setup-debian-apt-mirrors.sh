#!/bin/sh
# 构建 Debian 运行层前先换国内 apt 源。
# node:*-bookworm-slim / debian-slim 此时通常还没装 ca-certificates，
# 源必须走 http：写成 https 时 apt-get update 会在证书校验上失败，
# 有的 apt 仍返回 0，下一句就变成 Unable to locate package。

set -eu

. /etc/os-release
SUITE=${VERSION_CODENAME:-bookworm}

write_sources() {
  host=$1
  debian_uri="http://${host}/debian"
  security_uri="http://${host}/debian-security"
  if [ "$host" = "deb.debian.org" ]; then
    debian_uri="http://deb.debian.org/debian"
    security_uri="http://deb.debian.org/debian-security"
  fi

  cat > /etc/apt/sources.list.d/debian.sources << EOF
Types: deb
URIs: ${debian_uri}
Suites: ${SUITE} ${SUITE}-updates
Components: main
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg

Types: deb
URIs: ${security_uri}
Suites: ${SUITE}-security
Components: main
Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg
EOF

  if [ -f /etc/apt/sources.list ]; then
    : > /etc/apt/sources.list
  fi
}

ok=0
used=
for host in mirrors.cloud.tencent.com mirrors.aliyun.com mirrors.tuna.tsinghua.edu.cn mirrors.nju.edu.cn deb.debian.org; do
  echo "[daidai-apt] trying ${host}"
  write_sources "$host"
  rm -rf /var/lib/apt/lists/*
  if apt-get update \
    && apt-cache show openssh-client >/dev/null 2>&1 \
    && apt-cache show gosu >/dev/null 2>&1; then
    ok=1
    used=$host
    echo "[daidai-apt] using ${host}"
    break
  fi
  echo "[daidai-apt] ${host} unavailable or missing packages"
done

if [ "$ok" -ne 1 ]; then
  echo "[daidai-apt] no usable Debian mirror; openssh-client/gosu not in the package index" >&2
  exit 1
fi

echo "[daidai-apt] ready (${used}, ${SUITE})"
