# 管理员专属对话留存（第二候选版）

基于官方 v0.2.19，在独立 `feature/admin-conversation-archive` 分支开发。
源码基线标签：`development-baseline-v0.2.19`。
生产配置基线在 `/opt/sub2api` 的独立 Git 仓库：`production-before-conversation-archive`。
密码、上游凭据、数据库文件及备份均不得进入 Git。Git 不替代数据库备份。

## 范围与权限

- 捕获经过 API Key 认证的 JSON POST `/chat/completions`、`/responses`、`/messages`，包括网关已有相应别名。
- 普通用户不提供页面或读取 API。管理端 `/admin/conversation-archives` 要求管理员网页登录 JWT；管理员 API Key、受限管理员 Key 也不能读取。
- 查询列表、详情和删除沿用原管理员合规确认、认证、限流和操作审计，不绕过首次合规声明。
- 留存范围和管理员可见性需要向团队成员说明。第三方客户端自己的本地历史不受此功能控制。
- 不采集 WebSocket、Gemini 原生协议、音视频和图片生成接口。请求/响应中的内嵌媒体省略，不保存认证请求头。

## 数据含义

`conversation_archives` 每行保存一次客户端请求与对应的客户端可见响应，JSONB 保留结构和工具调用。
SSE 边转发边采集，不把整条响应攒完再发送。前端用文本插值显示 JSON，不执行模型生成的 HTML。
上游内部重试仍只保存一行；客户端主动重试算新请求。
每次传入的完整上下文是快照，不能简单拼接为新增消息，否则会重复历史。
客户端提供有效会话头时，可按用户、API Key 和会话 ID 组合筛选；不要只凭会话 ID 推断归属。
没有会话 ID 的请求独立展示，不推测合并。`previous_response_id` 的上游隐式历史不能自动完整还原。
记录 ID 为本平台留存标识，不保证与现有计费用量日志 request_id 相同。

## 开关与容量

```dotenv
SUB2API_ARCHIVE_IMAGE=sub2api-team:0.2.19-archive.3
CONVERSATION_ARCHIVE_ENABLED=true
CONVERSATION_ARCHIVE_MAX_BYTES=1048576
CONVERSATION_ARCHIVE_RETENTION_DAYS=30
```

默认关闭。最大保存量分别限制输入、输出，默认各 1 MiB，可配置 4 KiB–4 MiB。
超过输入上限不保留原始片段，输出仅保存有界 SSE 事件并标记受限；原始转发不截断。
响应中断、客户端取消、HTTP 错误分别标注。独立的有界 SSE 状态观察器持续跟踪完整响应，保存内容达到上限后仍可识别完成与失败；“内容受限”与请求状态独立展示。观察器兼容 LF、CRLF 和 CR 分隔，并保留早先出现的失败状态。
完成后的数据库写入有 2 秒超时，数据库不可用时保持网关响应并只记录无正文的错误日志。
首版属于尽力留存：进程崩溃、OOM、数据库写入失败会丢失在途记录，没有持久化重试队列，也不是全量审计保证。
正文中用户主动粘贴的密钥或其他敏感文字不保证能被识别；结构化认证字段会脱敏。

过期数据立即从查询隐藏。可由运维定时执行以下有界清理 SQL（先在测试库确认）；首版不自动安装 cron：

```sql
DELETE FROM conversation_archives WHERE id IN (
  SELECT id FROM conversation_archives WHERE expires_at <= NOW() ORDER BY expires_at LIMIT 500
);
```

## 测试与上线

1. 独立工具链容器运行 Go 测试和编译，前端运行类型检查、i18n 检查和构建。
   小内存服务器可在 `frontend/` 使用 `pnpm run build:archive`，完整类型检查与 Vite 打包串行运行，避免 Vite 插件额外并行启动第二份类型检查。
2. 临时 PostgreSQL `archive_test` 验证迁移、重复请求去重、用户过滤、过期、详情和删除；不要使用生产 DSN。
3. 测试通过后构建明确标签的自定义镜像；`deploy/docker-compose.archive.yml` 为可选覆盖文件。
4. 上线前执行生产 `/opt/sub2api/backup.sh`，复制覆盖文件并设置镜像及开关，仅重建应用容器。
5. 管理员访问 `/admin/conversation-archives`。用普通成员令牌验证 403，再做一笔授权上游流式调用核对正文。

## 回退

开发仅在 `/opt/sub2api-dev` 进行，生产镜像和数据未被这些源码修改自动替换。
若以后上线，需要恢复旧镜像时，先备份新增记录，然后用生产基线中的固定摘要镜像重建应用。
迁移 249 仅增加独立表和索引，回退应用时可保留该表，不要删除或覆盖数据库。
原始 `.env` 及其他敏感配置通过权限受限的部署备份恢复，不能从 Git 找回。
其他未来迁移不保证向后兼容；不要将本次回退方案套用于 PostgreSQL 跨主版本升级。

## 本次验证

- Go 新增采集、权限和正文解析测试通过，后端完整编译通过。
- 独立 PostgreSQL 18 测试库验证迁移、幂等保存、身份过滤、JSONB 正文读取、过期清理及删除通过，测试库已清理。
- 前端新增页面测试与中英文键完整性测试通过（5 项），新增页面 ESLint 通过。
- 全量 `vue-tsc --noEmit` 通过，Vite 生产打包通过。原构建插件会额外并行类型检查，故验证时按 `build:archive` 相同方式串行检查和打包，避免内存争用。
- 生产健康检查正常，现有 Codex 主进程保持运行。生产尚未启用此功能，真实上游端到端采集尚待使用授权凭据验证。

## 第二候选版验证与变更

- 修复长流式响应在保存达到上限后误判中断的问题，新增超过上限的完成、失败、客户端取消和多种换行边界测试。
- 识别 HTTP 200 中的结构化错误响应；保存 JSON 时保留大整数精度。
- 管理端增加开始时间（包含）与结束时间（不包含）筛选，API 使用带时区的 RFC3339 时间，浏览器本地时间转换为 UTC 后提交。
- 提问和回复按角色展示，支持 Chat Completions、Responses 和 Anthropic 的常见文本结构；多个 completion choice 分开显示，工具调用仍保留在可展开的原始结构中。所有内容通过文本插值显示。
- 修复编辑筛选项后直接翻页、迟到的详情响应等竞态。前端相关测试与文案完整性共 12 项通过，ESLint、全量类型检查和生产打包通过。
- 使用独立 PostgreSQL 和本地模拟上游验证 HTTP 流式转发、达到保存上限后完成状态、保存和管理员读取/删除，以及匿名/普通用户拒绝访问。该测试使用测试身份夹具，不替代真实 JWT 和服务商端到端验证。测试库已自动清理。
- 生产配置和 Codex 未修改。新增功能仍未上线；真实授权上游验证尚未完成，不能据此承诺绝对无 bug。

## GitHub 发布基线的测试夹具清理

上传排查时发现上游测试存在静态 PEM 样例；后续逐文件定位确认，本次密钥保护拦截的根因是内置 OAuth client_secret。这些 PEM 样例不是可解析的私钥；为避免静态密钥样式内容进入新仓库，支付、Vertex 及前端账号测试改为运行时生成、仅存于测试进程内的临时 RSA 密钥。生产运行逻辑没有因此修改，保护规则未关闭或绕过。

发布到 GitHub 的官方版本基线包含这项测试安全清理，并保留上游版本及许可；其源码树不再声称逐字节等同原始官方源码树。功能开发分支继续在该基线上提交。

受影响的支付 PEM 格式测试、Vertex 服务测试、后端 Vertex 入口测试均通过，前端账号编辑测试 73 项通过。由于完整 service 单元测试编译超过本机预留内存预算，Vertex 服务验证在隔离目录使用相同生产源码及受影响测试文件执行；没有宣称完整单元测试套件通过。

## 内置 OAuth 凭据移除

逐文件定位还确认，上游 Antigravity 与 Gemini CLI 模块及部署文档含有内置 OAuth client_secret，触发了仓库保护。已删除源码内置回退和文档中的凭据值，只接受显式配置的授权凭据：`ANTIGRAVITY_OAUTH_CLIENT_SECRET`、`GEMINI_CLI_OAUTH_CLIENT_SECRET`，或 Gemini 自定义 OAuth 客户端配置。缺失配置会返回明确错误，不再自动使用内置密钥。官方 API Key 接入不依赖这些 OAuth 配置。

相关 OAuth 模块及日志脱敏测试全部通过。此项加固会改变上述可选 OAuth 接入的默认行为，发布基线包含该安全修正，不能声称运行代码与原始官方版本完全相同。不要复用旧候选镜像作为这一安全版本；使用当前源码重新构建，例如 `docker build --build-arg VERSION=0.2.19-archive.3 -t sub2api-team:0.2.19-archive.3 .`。

## 远端历史与上线门槛

GitHub 的 main 是清理后的发布基线，feature/admin-conversation-archive 是功能分支。旧上游标签和原始历史仅在本机保留作回退，不要通过 --mirror 或 --tags 将含内置凭据的旧历史发布到新仓库。生产凭据及数据库备份仍只在服务器安全目录中保存。

本地目标测试和编译通过不等于绝对无 bug；生产上线还需要 GitHub CI、安全检查及真实授权上游验证通过。当前 GitHub 前端安全检查尚未通过，生产没有更新。
