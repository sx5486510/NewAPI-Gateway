# 文档架构规范

> 返回文档入口：[README.md](./README.md)

本文定义项目 Markdown 文档的完整信息架构、互链规则与维护流程，确保文档长期与代码一致。

## 1. 信息架构分层

### L0：仓库入口（对外）

- `README.md`（中文）
- `README.en.md`（英文）
- `THIRD_PARTY_NOTICES.md`（合规）

目标：让外部读者快速理解“项目是什么、如何启动、去哪里看细节”。

### L1：主线文档（落地上线）

- `QUICK_START.md`
- `ARCHITECTURE.md`
- `API_REFERENCE.md`
- `CONFIGURATION.md`
- `DEPLOYMENT.md`
- `OPERATIONS.md`

目标：覆盖从启动、接入到上线运维的完整链路。

### L2：研发文档（内部协作）

- `PROJECT_STRUCTURE.md`
- `DEVELOPMENT.md`
- `DATABASE_SCHEMA.md`

目标：帮助开发者快速定位代码、理解数据模型并安全迭代。

### L3：专题文档（局部深挖）

- `model-alias-manual-mapping.md`
- `FAQ.md`

目标：承载独立主题，避免主线文档过载。

## 2. 文档互链规则

### 顶部入口规则

`docs/` 下所有文档必须保留统一顶部入口：

```md
> 返回文档入口：[README.md](./README.md)
```

### 底部反向链接规则

每篇文档底部必须提供“相关文档”章节，至少包含 2 个链接。

### 主线阅读流

`QUICK_START -> ARCHITECTURE -> API_REFERENCE -> CONFIGURATION -> DEPLOYMENT -> OPERATIONS`

### 主题挂载规则

专题文档必须同时挂载到：

1. `docs/README.md` 的“完整文档清单”
2. 至少 1 篇主线文档的“相关文档”章节

## 3. 命名与内容边界

### 命名规范

- 文件名使用全大写英文蛇形：`API_REFERENCE.md`、`QUICK_START.md`
- 仅历史或专题文档允许小写命名（如 `model-alias-manual-mapping.md`）

### 内容边界

- `README.*`：总览与快速入口，不承载细节说明。
- `API_REFERENCE`：只写接口事实，不写实现推导。
- `ARCHITECTURE`：只写核心机制，不写完整接口表。
- `OPERATIONS`：只写值守动作和故障路径，不写开发流程。

## 4. 变更驱动更新规则

| 代码变更 | 必改文档 |
| --- | --- |
| `router/`、`controller/` 接口变化 | `API_REFERENCE.md` |
| `service/proxy.go`、`model/model_route.go` 策略变化 | `ARCHITECTURE.md`, `OPERATIONS.md` |
| `common/` 配置项变化 | `CONFIGURATION.md`, `DEPLOYMENT.md` |
| `model/` 字段或关系变化 | `DATABASE_SCHEMA.md` |
| 目录结构或模块职责变化 | `PROJECT_STRUCTURE.md`, `DEVELOPMENT.md` |

## 5. 新增文档流程

1. 在 `docs/` 新建文档并补顶部入口。
2. 在文档底部补“相关文档”链接。
3. 更新 `docs/README.md` 的文档清单与联动矩阵。
4. 若影响外部使用路径，同步更新 `README.md`（必要时 `README.en.md`）。

## 6. 文档质量检查清单

- 链接是否可点击且无死链。
- 标题是否与文件职责一致，避免“一篇文档覆盖多个主题”。
- 示例命令是否可在当前仓库结构执行。
- 配置默认值是否与代码一致。
- 是否存在“只从 A 指向 B，B 不回链 A”的孤岛关系。

## 相关文档

- 文档中心：[README.md](./README.md)
- 项目结构：[PROJECT_STRUCTURE.md](./PROJECT_STRUCTURE.md)
- 开发指南：[DEVELOPMENT.md](./DEVELOPMENT.md)
