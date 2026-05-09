# OAuth 提前限流与二开更新源设计

## 1. 背景

当前仓库已经具备两类相关能力，但都不完全满足本次目标：

1. OAuth 官方窗口限流
   - OpenAI OAuth 已有 `codex_5h_*` / `codex_7d_*` 窗口进度采样。
   - Anthropic OAuth / Setup Token 已有 `session_window_*`、`passive_usage_7d_*` 被动采样。
   - 官方窗口真正打满后，系统会通过账号级 `rate_limit_reset_at` 进入自动暂停，并在窗口重置后自动恢复。

2. 在线更新
   - 后端已有 `UpdateService`，部署脚本已有 `install.sh upgrade`。
   - 但更新源默认写死为上游 `Wei-Shaw/sub2api`，不适合二开版本长期维护。

本次要做两件事：

1. 为所有有官方 `5h` / `7d` 进度的 OAuth 账号增加“提前进入限流”的能力。
2. 把自动更新默认切换到 `moeacgx/sub2api`，保证二开版本可独立发布和在线升级。

## 2. 目标

### 2.1 OAuth 提前限流

- 支持 `5h` 和 `7d` 两个官方窗口。
- 支持两种阈值：
  - 百分比阈值：基于官方窗口使用率。
  - 金额阈值：基于系统统计的最近窗口实际成本（USD）。
- 支持账号单独配置。
- 支持分组兜底配置。
- 优先级规则固定为：
  - 账号优先，分组兜底。
  - 按窗口维度独立判断。
- 一旦命中阈值，复用现有账号级 `rate_limit_reset_at` 自动暂停链路。
- 到官方窗口 `reset_at` 后自动恢复。

### 2.2 二开更新源

- 在线更新和安装脚本默认从 `moeacgx/sub2api` 拉取版本。
- 支持配置覆盖更新仓库，避免再次写死。
- Docker 和二进制安装的文档说明统一切换到二开仓库。

## 3. 非目标

- 不实现“自动跟踪上游并自动合并发布”。
- 不改造为容器内自替换更新，Docker 仍使用拉新镜像/重启容器模式。
- 不移除现有 `window_cost_limit`、`quota_limit` 等旧能力。
- 不把提前限流改造成模型级 `model_rate_limits` 逻辑。

## 4. 术语

- 官方窗口：上游返回的 `5h` / `7d` 使用率和重置时间。
- 提前限流：尚未真正打满官方窗口前，由本地阈值主动触发的自动暂停。
- 本地提前限流状态：通过账号级 `rate_limit_reset_at` 表达，但附带额外标记，用于和真实上游限流区分。

## 5. 数据模型设计

## 5.1 账号级配置

账号级配置存入 `accounts.extra`，新增 4 个持久化键：

- `oauth_5h_preemptive_pause_percent`
- `oauth_5h_preemptive_pause_amount_usd`
- `oauth_7d_preemptive_pause_percent`
- `oauth_7d_preemptive_pause_amount_usd`

约束：

- 百分比：`> 0 && <= 100`
- 金额：`> 0`
- `null` / 缺省表示未配置

说明：

- 账号级按窗口独立生效。
- 例如只配置 `5h`，则 `7d` 仍可从分组兜底。

## 5.2 分组级配置

在 `groups` 表中新增 4 个显式字段：

- `oauth_5h_preemptive_pause_percent`
- `oauth_5h_preemptive_pause_amount_usd`
- `oauth_7d_preemptive_pause_percent`
- `oauth_7d_preemptive_pause_amount_usd`

类型与现有 `daily_limit_usd` / `weekly_limit_usd` / `monthly_limit_usd` 保持一致：

- `decimal(20,8)`
- `NULL` 表示未配置

对应需要同步修改：

- Ent schema
- Group service model
- admin group handler DTO
- 前端 `GroupsView` 创建/编辑表单

## 5.3 运行时标记

提前限流虽然复用账号级 `rate_limit_reset_at`，但必须额外打标，避免和真实上游限流混淆。

新增 `accounts.extra` 运行时键：

- `oauth_preemptive_pause_active`: `true`
- `oauth_preemptive_pause_window`: `"5h"` 或 `"7d"`
- `oauth_preemptive_pause_metric`: `"percent"` / `"amount"` / `"multiple"`
- `oauth_preemptive_pause_source`: `"account"` / `"group"`
- `oauth_preemptive_pause_reset_at`: RFC3339
- `oauth_preemptive_pause_triggered_at`: RFC3339

这些键不由前端编辑，只由服务端维护。

## 5.4 调度快照中的生效配置

调度热路径当前只可靠地拿到账号快照和分组 ID，不能在选账号时逐个现查分组配置。

因此需要在账号加载/快照构建阶段，预解析“分组兜底后的有效窗口阈值”，并带入账号快照。

建议在 `service.Account` 上新增 4 个非持久化字段：

- `EffectiveOAuth5hPreemptivePausePercent *float64`
- `EffectiveOAuth5hPreemptivePauseAmountUSD *float64`
- `EffectiveOAuth7dPreemptivePausePercent *float64`
- `EffectiveOAuth7dPreemptivePauseAmountUSD *float64`

计算规则：

1. 先看账号级配置。
2. 若账号级当前窗口某个维度未配，则从所属分组聚合。
3. 分组聚合采用最严格规则：
   - 百分比取最小值。
   - 金额取最小值。

说明：

- 这里的“账号优先”按窗口维度独立执行，不是“一旦账号配了任一值就整组窗口全部覆盖”。
- 例如账号只配置 `5h percent`，那 `5h amount`、`7d percent`、`7d amount` 仍可从分组兜底。

## 6. 官方窗口来源与适用范围

仅对能拿到官方窗口进度的 OAuth 账号生效。

### 6.1 OpenAI OAuth

使用现有字段：

- `codex_5h_used_percent`
- `codex_5h_reset_at`
- `codex_7d_used_percent`
- `codex_7d_reset_at`

### 6.2 Anthropic OAuth / Setup Token

5 小时窗口：

- 使用率：`session_window_utilization`（0-1，小数）
- 重置时间：`SessionWindowEnd`

7 天窗口：

- 使用率：`passive_usage_7d_utilization`（0-1，小数）
- 重置时间：`passive_usage_7d_reset`

说明：

- 本次需求口径是“所有有官方窗口进度的 OAuth 账号”。
- Anthropic `setup-token` 当前也走官方窗口采样，如果业务上后续要严格限定为纯 `oauth`，可再单独收窄；本次实现保持与现有官方窗口能力一致。

## 7. 核心行为设计

## 7.1 触发条件

每个窗口独立判断：

- `5h`
  - 百分比阈值：官方 `5h` 使用率达到阈值
  - 金额阈值：最近 `5h` 实际成本达到阈值

- `7d`
  - 百分比阈值：官方 `7d` 使用率达到阈值
  - 金额阈值：最近 `7d` 实际成本达到阈值

同一窗口内，只要任一阈值命中，即认为该窗口命中。

跨窗口规则：

- 若只命中 `5h`，暂停到 `5h reset_at`
- 若只命中 `7d`，暂停到 `7d reset_at`
- 若两个窗口都命中，暂停到更晚的 `reset_at`

## 7.2 金额统计口径

金额阈值使用系统统计的窗口实际成本（USD），不依赖上游是否回传剩余额度金额。

建议统一采用 `usage_logs` 聚合得到的账号窗口费用，口径与现有窗口费用限制保持一致：

- 使用账号窗口统计
- 使用标准成本字段
- 不按上游“名义额度金额”反推

## 7.3 暂停状态写入

命中提前限流时执行：

1. 调用现有 `SetRateLimited(accountID, resetAt)`
2. 同步写入运行时标记到 `accounts.extra`

这样可复用现有：

- `Account.IsRateLimited()`
- `Account.IsSchedulable()`
- 列表状态展示
- 到时间自动恢复

## 7.4 自动恢复

恢复主机制不新增后台任务，继续依赖现有逻辑：

- `rate_limit_reset_at` 过期后，调度自动把账号视为可调度
- 后续成功请求或状态刷新可继续清理残留运行态

需要补充的清理规则：

1. 当本地提前限流已过期时，运行时标记可在下次成功刷新时删除。
2. 当用户修改配置，导致本地提前限流不应继续生效时，只清理“本地提前限流”状态，不清理真实上游限流状态。

因此需要一个“仅清理本地提前限流”的服务方法，判断条件：

- `oauth_preemptive_pause_active == true`
- 且 `oauth_preemptive_pause_reset_at` 与当前 `rate_limit_reset_at` 一致

满足时才允许主动清除。

## 7.5 与真实上游限流的关系

优先级：

1. 真实上游 429 限流仍然有效
2. 本地提前限流只是在其之前抢先写入同一暂停通道

边界：

- 本地提前限流激活后，账号不会再发请求，因此一般不会再命中新的真实 429。
- 若极短时间内两者同时发生，取更晚的 `reset_at`，且保留真实限流优先语义。

## 8. 调度链路设计

## 8.1 插入位置

新增统一预检：

- `isAccountSchedulableForOAuthWindowPreemptivePause(...)`

调用顺序建议放在：

1. 基础状态判断
2. 模型可调度判断
3. API Key / Bedrock quota 判断
4. OAuth 官方窗口提前限流判断
5. 其它现有窗口费用/RPM等判断

原因：

- 该判断属于“是否要提前写入暂停状态”，应尽量在真正发起上游请求前完成。

## 8.2 热路径性能要求

禁止在选账号热路径中做逐账号现查分组配置、逐账号单独统计窗口成本。

性能策略：

1. 分组阈值在账号快照构建阶段预解析成 `Effective*` 字段。
2. 窗口金额统计走批量预取。

### 5h / 7d 金额批量预取

复用现有 `GetAccountWindowStatsBatch` 能力，新增一个适用于官方窗口的批量预取上下文：

- 对候选账号按“窗口开始时间”分桶
- 同窗口开始时间的账号批量查一次
- 结果写入上下文，后续调度判断直接读取

窗口开始时间计算：

- OpenAI 5h：`codex_5h_reset_at - 5h`
- OpenAI 7d：`codex_7d_reset_at - 7d`
- Anthropic 5h：`GetCurrentWindowStartTime()`
- Anthropic 7d：`passive_usage_7d_reset - 7d`

若缺少可用 `reset_at`，则该窗口金额阈值不触发，失败开放。

## 9. 前后端接口设计

## 9.1 账号接口

账号详情/列表 DTO 需新增 4 个配置字段：

- `oauth_5h_preemptive_pause_percent`
- `oauth_5h_preemptive_pause_amount_usd`
- `oauth_7d_preemptive_pause_percent`
- `oauth_7d_preemptive_pause_amount_usd`

建议同时暴露只读运行态字段：

- `oauth_preemptive_pause_active`
- `oauth_preemptive_pause_window`
- `oauth_preemptive_pause_metric`
- `oauth_preemptive_pause_source`
- `oauth_preemptive_pause_reset_at`

## 9.2 分组接口

分组 DTO、创建/更新接口新增 4 个字段：

- `oauth_5h_preemptive_pause_percent`
- `oauth_5h_preemptive_pause_amount_usd`
- `oauth_7d_preemptive_pause_percent`
- `oauth_7d_preemptive_pause_amount_usd`

## 10. 管理后台设计

## 10.1 账号编辑

在 OAuth 账号编辑区域新增“官方窗口提前限流”卡片：

- `5h`
  - 百分比阈值
  - 金额阈值（USD）
- `7d`
  - 百分比阈值
  - 金额阈值（USD）

文案要求明确：

- 命中后将提前进入自动暂停
- 恢复时间跟随官方窗口 `reset_at`
- 账号配置优先于分组配置

## 10.2 分组管理

在分组创建/编辑表单中新增相同配置区块，作为分组兜底策略。

仅在可能关联 OAuth 官方窗口账号的平台分组中显示。

## 10.3 状态展示

列表与状态徽标继续沿用现有“限流自动恢复”主表现。

补充一个轻量提示即可，例如：

- `提前限流（5h）`
- `提前限流（7d）`

不额外引入新的主状态类型。

## 11. 二开更新源设计

## 11.1 配置结构

扩展现有 `update` 配置：

```yaml
update:
  proxy_url: ""
  repo: "moeacgx/sub2api"
```

规则：

- 缺省值：`moeacgx/sub2api`
- 允许用户显式覆盖

## 11.2 后端 UpdateService

改造点：

1. 删除写死常量 `githubRepo = "Wei-Shaw/sub2api"`
2. `UpdateService` 持有 `repo string`
3. `CheckUpdate` / `PerformUpdate` / `Rollback` 相关流程都使用配置仓库
4. `UpdateInfo` 增加只读字段：
   - `repo`

## 11.3 install.sh

改造点：

- 默认 `GITHUB_REPO=moeacgx/sub2api`
- 允许环境变量覆盖
- `install / upgrade / rollback / list-versions` 全部使用同一仓库变量

## 11.4 Docker 与部署文档

更新以下文档/脚本中的默认仓库说明：

- `deploy/README.md`
- `deploy/docker-deploy.sh`
- `README.md`
- `README_CN.md`

策略：

- 说明 Docker 更新依旧是重新拉取你发布的镜像/源码版本
- 不承诺容器内二进制热替换

## 11.5 Release 约束

为了让在线更新真正可用，需要保持现有发布物格式：

- GitHub Releases
- 平台归档命名格式不变
- `checksums.txt` 继续生成

否则 `UpdateService` 无法匹配资产或校验下载内容。

## 12. 错误处理

- 缺失官方窗口数据：不触发提前限流，失败开放。
- 缺失 `reset_at`：不写入提前暂停，失败开放。
- 金额批量查询失败：不触发金额阈值，失败开放。
- 分组配置解析失败：忽略异常字段并记录日志，不阻断调度。
- 更新源配置非法：启动时报配置错误；后台更新检查返回明确 warning。

## 13. 测试策略

### 13.1 提前限流

- 账号级 `5h` 百分比触发
- 账号级 `5h` 金额触发
- 账号级 `7d` 百分比触发
- 账号级 `7d` 金额触发
- `5h` / `7d` 同时触发时取更晚 `reset_at`
- 账号优先于分组
- 多分组兜底取最严格
- 缺少官方窗口数据时失败开放
- 调高阈值后仅清理本地提前限流状态
- 本地提前限流不会误清真实上游限流

### 13.2 调度与快照

- 分组配置变更后触发 `group_changed`，相关账号快照更新
- 调度热路径不出现逐账号单查分组
- 金额预取批量查询按窗口开始时间分桶生效

### 13.3 更新源

- 默认检查仓库为 `moeacgx/sub2api`
- 覆盖配置时正确切换仓库
- `install.sh` 在默认和自定义仓库下都能获取版本列表
- `UpdateInfo.repo` 正确返回当前更新源

## 14. 实施顺序

1. 扩展配置和数据模型
2. 打通账号/分组前后端字段
3. 实现账号快照中的有效阈值解析
4. 实现 `5h` / `7d` 官方窗口提前限流预检
5. 实现本地提前限流运行态标记与定向清理
6. 更新管理后台表单与状态提示
7. 切换更新源到 `moeacgx/sub2api`
8. 更新安装脚本和部署文档
9. 补齐测试并验证

## 15. 风险与取舍

- 复用账号级 `rate_limit_reset_at` 是最贴近当前系统行为的方案，但必须额外打标，避免和真实上游限流混淆。
- 将分组阈值预解析进账号快照能保证热路径性能，但需要维护额外的派生字段同步。
- 金额阈值依赖本地 usage 统计，不等同于上游“官方剩余额度金额”，这是有意取舍，目的是保证跨平台一致性和可实现性。

## 16. 最终决策摘要

- 提前限流覆盖有官方 `5h` / `7d` 进度的 OAuth 账号。
- 支持百分比和金额两种阈值。
- 账号优先，分组兜底，分组按最严格聚合。
- 复用账号级 `rate_limit_reset_at` 自动暂停链路。
- 使用运行时标记区分“本地提前限流”和“真实上游限流”。
- 自动更新默认切到 `moeacgx/sub2api`，上游仓库仅用于手动合并。
