# 管理员专属对话留存（首版）

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
SUB2API_ARCHIVE_IMAGE=sub2api-team:0.2.19-archive.1
CONVERSATION_ARCHIVE_ENABLED=true
CONVERSATION_ARCHIVE_MAX_BYTES=1048576
CONVERSATION_ARCHIVE_RETENTION_DAYS=30
```

默认关闭。最大保存量分别限制输入、输出，默认各 1 MiB，可配置 4 KiB–4 MiB。
超过输入上限不保留原始片段，输出仅保存有界 SSE 事件并标记受限；原始转发不截断。
响应中断、客户端取消、HTTP 错误分别标注。捕获达到上限时无法判断后续流终止事件，可能标为中断，需结合“内容受限”标识查看。
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
