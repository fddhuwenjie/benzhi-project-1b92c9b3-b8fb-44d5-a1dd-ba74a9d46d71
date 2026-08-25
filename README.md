# 文物展柜环境异常处置台

这是面向博物馆值班员、文保技术员和展陈主管的本地工作台。系统把展柜传感器异常登记为可追踪事件，完成风险分级、责任分派、现场措施记录、复测、主管审批和证据归档关闭。数据保存在本地 JSON 快照与追加事件日志中。

标准构建、运行和测试命令：

```text
go test ./...
go run ./cmd/server -addr=127.0.0.1:19081
go run ./cmd/server --smoke -addr=127.0.0.1:19081
```

也可以使用 `PORT` 环境变量指定端口，服务会绑定到 `127.0.0.1:<PORT>`。浏览器访问服务根路径即可使用工作台；`MUSEUM_DATA_DIR` 可指定数据目录。

`GET /api/incidents` 支持 `status`、`risk`、`showcase`、`from` 和 `to`（RFC3339 或 `YYYY-MM-DD`）组合筛选，并返回 `incidents`、`stats` 与 `counts`。写入请求使用 `Idempotency-Key`；详情包含风险解释、处置截止状态、复测历史、证据时间线和归档编号。

登记入口同时接受单条载荷和 `records`/`items` 批次载荷；批次会全量校验后原子写入并返回 `batch_id`，重复键返回同一批次。登记结果包含同展柜、同文物、同指标的已关闭历史摘要。`GET /api/assignees/load?assignee=姓名` 可查看候选负责人未关闭任务负载；`GET /api/assignees/candidates` 返回按负载排序的候选和容量状态。批次可通过 `GET /api/batches/{batch_id}` 查询逐行异常及汇总。

公开处置接口包括 `POST /api/incidents/dispatch`、`POST /api/tasks/receive`、`POST /api/tasks/transfer`、`POST /api/tasks/action`、`POST /api/tasks/verify` 和 `POST /api/tasks/review`。首次分派后需由负责人确认接收，转派必须提供至少两个字的原因；归档编号可直接作为 `/api/incidents/{archive_id}` 查询。

`POST /api/incidents` 载荷带 `correction: true`、`incident_id`、`revision`、新读数和 `correction_reason` 时用于活动异常更正；原始读数和发现说明不会覆盖，更正前后值会进入时间线，已关闭异常返回归档状态。`GET /api/incidents/queue` 或列表查询的 `priority=true` 按风险、截止时间、接收超时和整改逾期计算只读处置队列，并支持 `assignee`、`status` 过滤和优先级聚合。

现场处置首次复测前可向 `POST /api/tasks/action/revise` 提交 `reason`、完整三类措施和附件索引，也可在现有 action 载荷中使用 `action: "revise"`；旧措施、附件和操作人会作为撤销快照留痕。`POST /api/tasks/verify` 支持 RFC3339 的 `measured_at`，会拒绝未来、倒序或不满足风险等级最小间隔的采样时间；缺省时保存服务当前时间。

现场处置的严格工作流要求 `categories` 中同时提供 `隔离`、`设备调整`、`保护措施` 三类措施；复测需连续两次合格才进入待复核，不合格读数会生成再处置提醒，并返回趋势、波动风险和接收/整改时效。主管退回会记录整改轮次，批准时执行时间线、附件索引和快照一致性校验；关闭后的 `archive_id` 可通过 `GET /api/archives/{archive_id}` 只读反查完整时间线、附件索引和处置耗时。
