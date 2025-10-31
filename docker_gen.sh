# 1. 先手动编译 docsgen
# 2. 准备一个干净目录，把当前仓库拷进去（排除 models-store）
mkdir -p /tmp/ctx
rsync -a --exclude='models-store' ./ /tmp/ctx/

# 3. 进入干净目录并生成
cd /tmp/ctx
/out/docsgen --formats "md,yaml" --source "cmd/cli/docs/reference"

# 4. 对比差异
git status --porcelain -- cmd/cli/docs/reference
git diff cmd/cli/docs/reference
