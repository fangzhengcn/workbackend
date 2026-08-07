#!/usr/bin/env bash
#
# 一键部署脚本 —— RBAC 权限管理后台
#
# 用法：
#   ./deploy.sh up        构建并启动全部服务（首次会自动生成 .env 密钥）
#   ./deploy.sh down      停止并移除容器（保留数据卷）
#   ./deploy.sh restart   重启服务
#   ./deploy.sh logs      跟踪查看日志
#   ./deploy.sh ps        查看服务状态
#   ./deploy.sh rebuild   强制重新构建镜像并启动
#   ./deploy.sh init      仅生成 .env，不启动
#   ./deploy.sh destroy   停止并删除数据卷（**会清空数据库**）

set -euo pipefail

cd "$(dirname "$0")"

ENV_FILE=".env"

# ---- 输出着色 ----
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly BLUE=$'\033[0;34m'
readonly NC=$'\033[0m'

info()  { printf '%s\n' "${BLUE}▸${NC} $*"; }
ok()    { printf '%s\n' "${GREEN}✓${NC} $*"; }
warn()  { printf '%s\n' "${YELLOW}!${NC} $*"; }
die()   { printf '%s\n' "${RED}✗${NC} $*" >&2; exit 1; }

# ---- 前置检查 ----
check_prerequisites() {
  command -v docker >/dev/null 2>&1 || die "未安装 Docker，请先安装：https://docs.docker.com/get-docker/"

  # 优先用 compose v2（docker compose），回退到 v1（docker-compose）
  if docker compose version >/dev/null 2>&1; then
    COMPOSE=(docker compose)
  elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE=(docker-compose)
  else
    die "未找到 docker compose，请升级 Docker 或安装 docker-compose"
  fi

  docker info >/dev/null 2>&1 || die "Docker 守护进程未运行，请先启动 Docker"
}

# ---- 生成随机密钥 ----
random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    # 无 openssl 时退回 /dev/urandom
    od -An -tx1 -N32 /dev/urandom | tr -d ' \n'
  fi
}

random_password() {
  # 数据库密码避免特殊字符，防止在 DSN / shell 中引发转义问题
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 24 | tr -dc 'A-Za-z0-9' | head -c 24
  else
    od -An -tx1 -N16 /dev/urandom | tr -d ' \n'
  fi
}

# ---- 初始化 .env ----
init_env() {
  if [[ -f "$ENV_FILE" ]]; then
    ok "$ENV_FILE 已存在，跳过生成（如需重建请先手动删除）"
    return
  fi

  info "生成 $ENV_FILE 与随机密钥..."

  local mysql_password redis_password jwt_secret aes_key hmac_key
  mysql_password="$(random_password)"
  redis_password="$(random_password)"
  jwt_secret="$(random_hex)"
  aes_key="$(random_hex)"
  hmac_key="$(random_hex)"

  cat > "$ENV_FILE" <<EOF
# 由 deploy.sh 自动生成于 $(date '+%Y-%m-%d %H:%M:%S')
# 本文件含生产密钥，已被 .gitignore 忽略，切勿提交或外传。
#
# 重要：AES/HMAC 密钥一旦丢失，已加密的邮箱/手机号将无法解密。
#       请务必备份本文件到安全位置（密码管理器 / KMS）。

# ---- MySQL ----
MYSQL_ROOT_PASSWORD=${mysql_password}
MYSQL_DATABASE=rbac_admin

# ---- Redis ----
REDIS_PASSWORD=${redis_password}

# ---- 前端对外端口 ----
WEB_PORT=80

# ---- JWT 签名密钥 ----
APP_JWT_SECRET=${jwt_secret}

# ---- 敏感字段加密密钥（AES-256-GCM / HMAC 盲索引）----
APP_CRYPTO_AESKEY=${aes_key}
APP_CRYPTO_HMACKEY=${hmac_key}

# ---- CORS 白名单（逗号分隔）----
# 生产环境必须改为真实前端域名；留空则使用 config/production/app.yaml 中的值
APP_CORS_ALLOWORIGINS=
EOF

  chmod 600 "$ENV_FILE"
  ok "$ENV_FILE 已生成（权限 600）"
  warn "请备份 $ENV_FILE —— 丢失 AES/HMAC 密钥将导致已加密数据无法解密"
}

# ---- 等待服务健康 ----
wait_for_health() {
  local port="${WEB_PORT:-80}"
  info "等待服务就绪..."

  for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; then
      ok "服务已就绪"
      return 0
    fi
    sleep 2
  done

  warn "健康检查超时。请执行 ./deploy.sh logs 查看日志排查"
  return 1
}

# ---- 打印部署结果 ----
print_summary() {
  local port
  port="$(grep -E '^WEB_PORT=' "$ENV_FILE" | cut -d= -f2)"
  port="${port:-80}"

  echo
  ok "部署完成"
  echo
  printf '  前端地址   http://localhost:%s\n' "$port"
  printf '  健康检查   http://localhost:%s/healthz\n' "$port"
  printf '  初始账号   admin / 123456\n'
  echo
  warn "首次登录后请立即修改 admin 密码（种子数据中的密码是公开示例值）"
  echo
  printf '  查看日志   ./deploy.sh logs\n'
  printf '  停止服务   ./deploy.sh down\n'
  echo
}

# ---- 主命令 ----
cmd_up() {
  init_env
  info "构建镜像并启动服务（首次构建约需数分钟）..."
  "${COMPOSE[@]}" up -d --build
  wait_for_health || true
  print_summary
}

cmd_rebuild() {
  init_env
  info "强制重新构建镜像..."
  "${COMPOSE[@]}" build --no-cache
  "${COMPOSE[@]}" up -d
  wait_for_health || true
  print_summary
}

cmd_down() {
  info "停止并移除容器（数据卷保留）..."
  "${COMPOSE[@]}" down
  ok "已停止"
}

cmd_destroy() {
  warn "此操作将删除数据卷，数据库内所有数据都会丢失！"
  read -r -p "确认删除？输入 yes 继续: " reply
  [[ "$reply" == "yes" ]] || { info "已取消"; exit 0; }
  "${COMPOSE[@]}" down -v
  ok "容器与数据卷已删除"
}

main() {
  check_prerequisites

  case "${1:-up}" in
    up)       cmd_up ;;
    rebuild)  cmd_rebuild ;;
    down)     cmd_down ;;
    restart)  "${COMPOSE[@]}" restart; ok "已重启" ;;
    logs)     "${COMPOSE[@]}" logs -f --tail=100 ;;
    ps)       "${COMPOSE[@]}" ps ;;
    init)     init_env ;;
    destroy)  cmd_destroy ;;
    *)
      sed -n '3,20p' "$0" | sed 's/^# \{0,1\}//'
      exit 1
      ;;
  esac
}

main "$@"
