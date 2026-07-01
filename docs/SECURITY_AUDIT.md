# LLM 安全审计功能

## 概述

自动安全审计系统会实时分析所有 LLM API 请求和响应，检测以下安全风险：

- **提示词注入攻击**（Prompt Injection / Jailbreak）
- **危险操作**（文件删除、命令执行、网络攻击等）
- **敏感数据泄露**（API 密钥、私钥、信用卡、数据库连接串等）
- **异常工具调用**（过量调用、可疑工具、重复错误等）

## 数据库字段

### `llm_trace` 表新增字段

| 字段名 | 类型 | 说明 |
|--------|------|------|
| `risk_level` | varchar(32) | 风险等级：`safe`、`low`、`medium`、`high`、`critical`、`unknown` |
| `risk_tags` | text | 检测到的风险标签 JSON 数组，如 `["prompt_injection", "api_key_leak"]` |
| `auto_reviewed` | boolean | 是否已完成自动审计 |

## 风险等级说明

| 等级 | 说明 | 示例 |
|------|------|------|
| `safe` | 安全，未检测到任何风险 | 正常对话 |
| `low` | 低危，可能需要关注 | 重复的工具错误 |
| `medium` | 中危，建议审查 | 过量工具调用、批量邮箱/手机号 |
| `high` | 高危，需要重点关注 | 指令覆盖、命令执行、数据库连接串、身份证号 |
| `critical` | 严重，立即处理 | 提示词注入、危险文件操作、API 密钥泄露、私钥泄露 |

## 检测规则

### 1. 提示词注入检测

检测关键词和模式：
- `ignore previous instructions`、`disregard previous`
- `DAN mode`、`developer mode`、`jailbreak`
- `你是`、`忽略之前`、`无视规则`
- 指令覆盖模式：`override safety`、`bypass restriction`

**风险等级**：`high`  
**标签**：`prompt_injection`、`instruction_override`

### 2. 危险操作检测

#### 文件系统操作
- `rm -rf`、`rmdir /s`、`format c:`、`dd if=`
- **风险等级**：`critical`
- **标签**：`dangerous_file_operation`

#### 命令执行
- `eval(`、`exec(`、`system(`、`shell_exec`
- `curl -o`、`wget -o`、`powershell -encodedcommand`
- **风险等级**：`high`
- **标签**：`command_execution`

#### 网络攻击
- `nmap`、`sqlmap`、`metasploit`、`port scan`
- **风险等级**：`high`
- **标签**：`network_attack`

#### 数据库操作
- `drop database`、`drop table`、`truncate table`
- `union select`、`' or '1'='1`
- **风险等级**：`medium`
- **标签**：`sql_operation`

### 3. 敏感数据检测

#### API 密钥和令牌
- OpenAI API key: `sk-[a-zA-Z0-9-_]{20,}`
- GitHub token: `ghp_[a-zA-Z0-9]{36,}`
- AWS access key: `AKIA[0-9A-Z]{16}`
- JWT token: `eyJ...`
- **风险等级**：`critical`
- **标签**：`api_key_leak`

#### 私钥
- `BEGIN RSA PRIVATE KEY`
- `BEGIN OPENSSH PRIVATE KEY`
- **风险等级**：`critical`
- **标签**：`private_key_leak`

#### 信用卡号
- 基础 Luhn 校验格式
- **风险等级**：`critical`
- **标签**：`credit_card`

#### 数据库连接串
- `mysql://`、`postgresql://`、`mongodb://`
- `Server=`、`User ID=`
- **风险等级**：`high`
- **标签**：`db_connection_string`

#### 批量 PII 数据
- 5+ 邮箱地址
- 5+ 手机号
- **风险等级**：`medium`
- **标签**：`multiple_emails`、`multiple_phones`

#### 身份证号
- 中国身份证号格式（18位）
- **风险等级**：`high`
- **标签**：`id_card_number`

### 4. 异常工具调用检测

#### 过量工具调用
- 单次响应超过 10 个工具调用
- **风险等级**：`medium`
- **标签**：`excessive_tool_calls`

#### 可疑工具名称
- `execute_code`、`run_command`、`eval_code`
- `file_delete`、`file_write`、`database_query`
- **风险等级**：`high`
- **标签**：`suspicious_tool_call`

#### 重复工具错误
- 单次请求超过 5 个 `"error"` 字段
- **风险等级**：`low`
- **标签**：`repeated_tool_errors`

## API 使用

### 查询审计记录

```http
GET /api/llm-trace/?p=0&page_size=15&risk_level=high&status=all
```

**查询参数**：
- `p`: 页码（从 0 开始）
- `page_size`: 每页条数
- `risk_level`: 风险等级过滤（`all`、`safe`、`low`、`medium`、`high`、`critical`）
- `status`: 状态过滤（`all`、`success`、`error`）
- `keyword`: 关键词搜索
- `provider`: 供应商名称
- `model`: 模型名称

### 响应示例

```json
{
  "success": true,
  "data": {
    "items": [
      {
        "id": 123,
        "request_id": "req_abc123",
        "model_name": "gpt-4",
        "provider_name": "openai",
        "status_code": 200,
        "risk_level": "high",
        "risk_tags": "[\"prompt_injection\",\"api_key_leak\"]",
        "auto_reviewed": true,
        "created_at": 1719820800
      }
    ],
    "total": 100,
    "page": 0,
    "page_size": 15
  }
}
```

### 查看详情

```http
GET /api/llm-trace/{id}
```

返回完整的请求体、响应体和审计结果。

## 前端展示

### 列表页

- 风险等级筛选器（下拉框）
- 风险等级徽章（彩色标签）
- 风险标签列表（每个检测到的风险类型）
- 警告图标（高危/严重记录）

### 详情页

- 安全审计结果卡片（顶部展示）
  - 风险等级大徽章
  - 所有风险标签
- 请求内容
- 响应内容
- 错误信息（如有）

## 配置

### 开启/关闭审计

审计功能通过 `common.LLMTraceEnabled` 配置项控制：

```go
// common/init.go
LLMTraceEnabled = os.Getenv("LLM_TRACE_ENABLED") == "true"
```

环境变量：
```bash
LLM_TRACE_ENABLED=true
```

### 自定义检测规则

编辑 `service/security_audit.go`，在对应的检测函数中添加/修改规则：

- `checkPromptInjection()` - 提示词注入
- `checkDangerousActions()` - 危险操作
- `checkSensitiveData()` - 敏感数据
- `checkAbnormalToolCalls()` - 工具调用

## 数据库迁移

自动迁移会在启动时执行，添加以下字段：

```go
// model/main.go: runMigrations()
- risk_level (varchar(32), 默认 'unknown')
- risk_tags (text)
- auto_reviewed (boolean, 默认 false)
```

## 测试

运行安全审计测试：

```bash
go test ./service -run TestAuditLLMContent -v
```

测试覆盖：
- 提示词注入检测（3 个用例）
- 危险操作检测（3 个用例）
- 敏感数据检测（5 个用例）
- 工具调用检测（1 个用例）
- JSON 文本提取（4 个用例）

## 性能影响

- 每次审计耗时：< 5ms（正则匹配 + JSON 解析）
- 内存开销：约 1-2KB per request
- 数据库影响：3 个额外字段，索引 `risk_level`

## 注意事项

1. **隐私保护**：审计日志包含完整请求/响应内容，注意定期清理
2. **误报**：正则规则可能产生误报，建议人工复核高危记录
3. **性能**：大规模部署建议采用异步审计（后台队列）
4. **规则更新**：根据实际攻击模式定期更新检测规则

## 未来改进

- [ ] 支持自定义检测规则配置（YAML/JSON）
- [ ] 异步审计队列（减少请求延迟）
- [ ] 机器学习模型检测（更准确的注入识别）
- [ ] 风险告警通知（Webhook/邮件/Slack）
- [ ] 审计日志导出（CSV/Excel）
- [ ] 风险趋势分析仪表盘
