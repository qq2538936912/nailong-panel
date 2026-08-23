# Docker 快速拉取

拉取镜像用 `pull` + `up -d`，**不要加 `--build`**（加了会本地构建，不是纯拉取）。

默认拉取 `xiaofeilong2/panel:latest`（或在 `.env` 里设置的 `PANEL_IMAGE`）。

## Alpine（默认）

```bash
docker compose pull
docker compose up -d
```

## Debian

```bash
docker compose -f docker-compose.debian.yml pull
docker compose -f docker-compose.debian.yml up -d
```

或一条命令指定镜像：

```bash
PANEL_IMAGE=xiaofeilong2/panel:debian docker compose pull
PANEL_IMAGE=xiaofeilong2/panel:debian docker compose up -d
```

## 只拉镜像、不启动

```bash
docker pull xiaofeilong2/panel:debian
```

## 常用标签

```bash
docker pull xiaofeilong2/panel:latest          # Alpine 精简版
docker pull xiaofeilong2/panel:debian-full    # Debian 完整版（带 Go、编译链）
```

## 注意

- 镜像需在仓库已发布；若 `xiaofeilong2/panel` 尚未推到 Docker Hub / 镜像站，`pull` 会 404，此时只能 `--build` 本地编，或改 `PANEL_IMAGE` 指向上游镜像（如 `linzixuanzz/daidai-panel:debian`）。
- 数据目录为 `./Panel`；Alpine ↔ Debian 切换时不要加 `-v` 删卷，数据可保留。
