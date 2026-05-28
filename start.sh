export PATH="/usr/local/go/bin:$PATH"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

set -a          # 之后定义的变量都会 export 到子进程
source .env     # 把 .env 读进当前 shell
set +a
echo "正在编译并启动（首次约 30–60 秒无输出属正常，请勿 Ctrl+C）..."
go run ./cmd/claw
