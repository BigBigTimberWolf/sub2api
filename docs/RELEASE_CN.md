# 发布流程说明

本文档记录当前仓库的正式发布流程。**当前发布链路仅上传 GitHub Release 及其附件，不再推送 GHCR、DockerHub 镜像，也不再执行版本文件回写或 Telegram 通知。**

## 一、发布方式概览

当前发布流程由以下文件定义：

- `.github/workflows/release.yml`
- `.goreleaser.yaml`

发布触发方式有两种：

1. 推送符合 `v*` 规则的标签，例如 `v0.1.0`
2. 在 GitHub Actions 页面手动运行 `Release` 工作流，并填写目标标签

当前工作流只声明了最小必需权限：

```yaml
permissions:
  contents: write
```

这意味着当前发布流程的核心目标是：

- 构建前端产物
- 编译后端二进制
- 生成归档包与校验文件
- 创建或更新 GitHub Release
- 上传 Release 附件

## 二、发布产物

当前 GoReleaser 会生成以下类型的产物：

- Linux `amd64` / `arm64`
- Windows `amd64`
- macOS `amd64` / `arm64`

产物命名规则如下：

```text
sub2api_<版本号>_<系统>_<架构>
```

其中：

- Linux、macOS 默认输出 `tar.gz`
- Windows 输出 `zip`
- 同时会生成 `checksums.txt`

归档中会包含：

- 可执行文件
- `LICENSE*`
- `README*`
- `deploy/*`

## 三、发布前置条件

正式发布前，建议先确认以下事项：

1. GitHub 仓库已启用 Actions
2. 仓库默认 `GITHUB_TOKEN` 具备 `contents: write`
3. 本地工作区干净，没有未提交改动
4. 本地 `main` 已同步远端最新提交
5. 本次发布对应的代码已经先推送到远端分支

推荐先执行：

```bash
git fetch origin
git checkout main
git rebase origin/main
git status
```

## 四、推荐发布步骤

### 1. 提交并推送待发布代码

先确保本次发布代码已经提交并推送到远端：

```bash
git push origin main
```

### 2. 创建注释标签

当前工作流会读取**注释标签正文**作为 Release 内容，因此推荐始终使用**注释标签**：

```bash
git tag -a v0.1.0 -m "v0.1.0

- 发布说明第 1 条
- 发布说明第 2 条
"
```

说明：

- 标签名必须以 `v` 开头，否则不会触发发布工作流
- 第一行通常作为标签标题
- 从第二行开始的正文会进入 GitHub Release 内容

### 3. 推送标签

```bash
git push origin v0.1.0
```

推送成功后，GitHub Actions 会自动触发 `Release` 工作流。

### 4. 检查发布结果

发布后请检查：

1. GitHub Actions 中 `Release` 工作流是否成功
2. GitHub Releases 页面是否生成对应版本
3. Release 附件是否齐全
4. `checksums.txt` 是否已上传

## 五、手动触发发布

如果需要手动重跑发布流程，可以在 GitHub Actions 页面手动运行 `Release` 工作流。

注意事项：

1. 手动触发时填写的 `tag` 必须已经存在于仓库中
2. 手动触发不会替你创建标签
3. 如果标签不存在，工作流中的检出与标签说明读取阶段会失败

## 六、发布说明写法建议

因为工作流会读取标签正文作为 Release 内容，建议使用下面的格式：

```text
v0.1.0

- 新增：说明一
- 修复：说明二
- 调整：说明三
```

推荐写法：

- 第一行只写版本号或简短标题
- 第二行留空
- 正文使用短条目，便于直接展示在 Release 页面

## 七、常见风险与注意事项

### 1. 不要先推标签、后推代码

标签应当始终指向**最终要发布的提交**。推荐顺序是：

1. 先推送 `main`
2. 再创建并推送标签

如果先推了标签，再去改写分支历史，容易出现：

- Release 基于旧提交生成
- 标签与远端 `main` 最新代码不一致

### 2. 不要强推覆盖远端主分支

如果 `git push origin main` 被拒绝，说明远端分支比本地更新。应先同步远端，再处理冲突或 rebase，避免直接覆盖远端历史。

### 3. 当前不会发布容器镜像

当前流程**不会**发布以下内容：

- GHCR 镜像
- DockerHub 镜像
- 多架构镜像 manifest

如果后续需要恢复镜像发布，必须重新修改：

- `.github/workflows/release.yml`
- `.goreleaser.yaml`

### 4. 当前不会自动回写版本文件

当前流程中的 `backend/cmd/server/VERSION` 只在工作流内部用于构建时注入版本，不会在发布成功后自动提交回默认分支。

## 八、最小权限说明

当前发布链路只需要：

```yaml
contents: write
```

当前不再需要：

- `packages: write`
- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`
- `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_CHAT_ID`

## 九、排查建议

### 标签推送后没有触发发布

优先检查：

1. 标签是否为 `v*` 格式
2. 是否推送到了正确仓库
3. 仓库 Actions 是否启用

### Release 内容为空

优先检查：

1. 是否使用了注释标签
2. 标签正文是否为空
3. 是否只有标题行、没有正文

### 手动触发失败

优先检查：

1. 输入的 `tag` 是否真实存在
2. 标签是否已经推送到远端

## 十、参考文件

如需核对当前真实流程，请直接以仓库配置为准：

- [release.yml](/C:/Users/Administrator/code/数字员工/代码/sub2api/.github/workflows/release.yml)
- [.goreleaser.yaml](/C:/Users/Administrator/code/数字员工/代码/sub2api/.goreleaser.yaml)
