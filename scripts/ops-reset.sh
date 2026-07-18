#!/bin/bash
# 在 GCP 网页 SSH 中执行：完整停服 → 拉取 → 安装 → 重启 → 自检
# 用法：
#   bash /opt/find-assets/src/scripts/ops-reset.sh
# 或从本机复制粘贴整段到网页 SSH。
set -euo pipefail

ROOT=/opt/find-assets
SRC="$ROOT/src"
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
  SUDO="sudo"
fi

echo "======== 1) 停掉已运行的 crypto-scanner ========"
$SUDO systemctl stop crypto-scanner || true
$SUDO systemctl disable crypto-scanner || true
# 兜底杀残留进程（oneshot 批跑一般已退出）
pkill -f '/opt/find-assets/crypto-scanner' 2>/dev/null || true
pkill -f '/opt/find-assets/scanner' 2>/dev/null || true
sleep 1
echo "service: $($SUDO systemctl is-active crypto-scanner || true)"
ps aux | grep -E 'crypto-scanner|/scanner' | grep -v grep || echo "(无相关进程)"

echo "======== 2) 拉取最新代码 ========"
cd "$SRC"
git fetch origin
git reset --hard origin/main
git log -1 --oneline

echo "======== 3) 安装二进制 ========"
if [ -f "$SRC/crypto-scanner" ]; then
  cp "$SRC/crypto-scanner" "$ROOT/crypto-scanner"
  chmod +x "$ROOT/crypto-scanner"
  file "$ROOT/crypto-scanner"
else
  echo "ERROR: $SRC/crypto-scanner 不存在，请先本机交叉编译并 push"
  exit 1
fi

if [ -f "$SRC/scanner" ]; then
  cp "$SRC/scanner" "$ROOT/scanner"
  chmod +x "$ROOT/scanner"
  file "$ROOT/scanner"
fi

if [ -d "$SRC/scripts/ashare" ]; then
  chmod +x "$SRC/scripts/ashare"/*.sh 2>/dev/null || true
  mkdir -p "$ROOT/scripts/ashare" "$ROOT/logs"
  cp -f "$SRC/scripts/ashare"/*.sh "$ROOT/scripts/ashare/" 2>/dev/null || true
  chmod +x "$ROOT/scripts/ashare"/*.sh 2>/dev/null || true
fi

echo "======== 4) 确认 .env / systemd ========"
if [ ! -f "$ROOT/.env" ]; then
  echo "WARN: $ROOT/.env 不存在，邮件会跳过。需要时再 tee 写入 FIND_ASSETS_SMTP_PASS"
else
  ls -l "$ROOT/.env"
fi

if [ ! -f /etc/systemd/system/crypto-scanner.service ]; then
  echo "创建默认 crypto-scanner.service ..."
  $SUDO tee /etc/systemd/system/crypto-scanner.service >/dev/null <<'EOF'
[Unit]
Description=find-assets crypto scanner
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/find-assets
EnvironmentFile=/opt/find-assets/.env
ExecStart=/opt/find-assets/crypto-scanner -source=okx
Restart=always
RestartSec=30

[Install]
WantedBy=multi-user.target
EOF
fi

echo "======== 5) 重新启用并启动 ========"
$SUDO systemctl daemon-reload
$SUDO systemctl enable --now crypto-scanner
sleep 2

echo "======== 6) 健康检查 ========"
echo "=== 服务 ===" && $SUDO systemctl is-active crypto-scanner
echo "=== 最近日志 ===" && $SUDO journalctl -u crypto-scanner -n 20 --no-pager
echo "=== 缓存 ===" && ls -la "$ROOT/crypto/pools/" 2>/dev/null || echo "暂无（首轮触发前正常）"
echo "=== cron（A股，若已配置）==="
crontab -l 2>/dev/null || echo "(当前用户无 crontab；A股需按文档 §12.4 另行配置)"
echo "DONE."
