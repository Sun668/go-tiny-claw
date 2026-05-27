set -a          # 之后定义的变量都会 export 到子进程
source .env     # 把 .env 读进当前 shell
set +a
go run ./cmd/claw