# CPA 额度查询与冷却重置设计

## 目标

Gateway 的 CPA 认证文件页面为每个运行时认证提供两个明确分离的操作：

- “获取真实额度”通过 CPA `POST /v0/management/api-call` 查询服务商额度。
- “重置冷却”通过 CPA `POST /v0/management/reset-quota` 清理 CPA 路由冷却/额度标记。

Gateway 不创建新的额度业务接口，也不自行处理或保存服务商 Token。

## 接口边界

认证文件的 `auth_index` 一律来自 CPA `GET /v0/management/auth-files`。真实额度查询继续复用当前与 CPA 官方 Management Center 一致的 provider 请求模板，CPA 负责 `$TOKEN$` 替换、Token 刷新和代理选择。

冷却重置请求体固定为：

```json
{"auth_index":"<runtime auth index>"}
```

成功响应沿用 CPA 原始结构。重置不会解除用户手动设置的 disabled 状态。

## Gateway 代理行为

`api-call` 与 `reset-quota` 都是运行时操作：成功时不得持久化 CPA 配置快照，也不得安排 Provider 同步。其他成功的管理写请求继续保持现有持久化语义。

`api-call` 专用代理拒绝超过 1 MiB 的请求体并返回 413；与 CPA 的 60 秒请求超时对齐，Gateway 等待响应头的时间设为 65 秒；传输失败返回稳定错误信息，不泄露底层地址；响应不转发 hop-by-hop headers。

## 前端行为

每个有 `auth_index` 的认证显示“重置冷却”。支持额度查询且未禁用的认证显示“获取真实额度”。两个动作分别维护 in-flight 状态，重复点击不会重复提交。

批量查询真实额度最多并行四个认证。单个失败只更新对应认证的错误状态，不中断其他认证。

Gateway 管理请求必须把全局 Axios 拦截器返回的 `{success:false,message}` 恢复为异常，避免把 CPA 外层错误误判为额度空响应。provider 返回成功但不包含任何可展示额度项目时按无效响应处理。

## 验收

- 冷却重置请求准确携带字符串 `auth_index`，成功后刷新认证列表。
- 冷却重置与真实额度查询都不触发快照持久化或 Provider 同步。
- 批量额度查询并发数不超过四。
- CPA 外层错误、provider 内层错误和空额度响应都显示可读错误。
- 代理边界、前端交互和现有 provider 适配器测试全部通过。
