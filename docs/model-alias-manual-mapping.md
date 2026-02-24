# 模型别名手动映射

> 返回文档入口：[README.md](./README.md)

## 文档导航

- 上一篇：[ARCHITECTURE.md](./ARCHITECTURE.md)
- 下一篇：[API_REFERENCE.md](./API_REFERENCE.md)
- 运维排障：[OPERATIONS.md](./OPERATIONS.md)

当自动归一化仍无法满足路由需求时，可在供应商详情页配置“模型别名映射”。

## 配置位置

- 数据库表：`providers`
- 字段：`model_alias_mapping`
- 值格式：JSON 对象，结构为 `{"保留模型名":"该供应商上游真实模型名"}`
- 记忆方式：**前面保留，后面折叠**。
  - 前面的 key 是你希望在路由界面/调用侧看到的名字。
  - 后面的 value 是上游真实名字（可很乱，比如 `bbbxxxcccddd`）。

## 示例

```json
{
  "aaa": "bbbxxxcccddd",
  "gpt-oss-120b": "[反重力]gpt-oss-120b"
}
```

## 管理 API

- 读取：`GET /api/provider/:id/model-alias-mapping`
- 更新：`PUT /api/provider/:id/model-alias-mapping`

请求体示例：

```json
{
  "model_alias_mapping": {
    "aaa": "bbbxxxcccddd"
  }
}
```

## SQLite 修改示例

```sql
UPDATE providers
SET model_alias_mapping='{"aaa":"bbbxxxcccddd"}'
WHERE id=1;
```

更新后新请求会立即生效，无需重启服务。路由界面会将 `bbbxxxcccddd` 聚合显示到 `aaa` 组中。

## 相关文档

- 架构说明：[ARCHITECTURE.md](./ARCHITECTURE.md)
- API 参考：[API_REFERENCE.md](./API_REFERENCE.md)
- 运维手册：[OPERATIONS.md](./OPERATIONS.md)
