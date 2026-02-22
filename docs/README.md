# 文档中心

> 入口说明：这里是项目文档导航页，建议先读本页再进入各专题文档。

## 文档导航

| 主题 | 文档 | 适合人群 | 目标 |
| --- | --- | --- | --- |
| 快速启动 | [QUICK_START.md](./QUICK_START.md) | 初次部署者、测试者 | 10 分钟跑通网关 |
| 系统架构 | [ARCHITECTURE.md](./ARCHITECTURE.md) | 后端开发、架构师 | 理解请求链路、路由算法与同步机制 |
| 配置说明 | [CONFIGURATION.md](./CONFIGURATION.md) | 运维、部署工程师 | 明确环境变量、启动参数与配置优先级 |
| 部署指南 | [DEPLOYMENT.md](./DEPLOYMENT.md) | 运维、SRE | 本地、Docker、反向代理、systemd 部署 |
| API 参考 | [API_REFERENCE.md](./API_REFERENCE.md) | 前端、后端、平台接入方 | 查看认证、接口分组、请求示例、错误码 |
| 开发指南 | [DEVELOPMENT.md](./DEVELOPMENT.md) | 项目贡献者 | 本地开发、调试、扩展点与提交流程 |
| 项目结构 | [PROJECT_STRUCTURE.md](./PROJECT_STRUCTURE.md) | 新成员 | 快速定位模块与职责边界 |
| 数据模型 | [DATABASE_SCHEMA.md](./DATABASE_SCHEMA.md) | 后端、数据分析、运维 | 理解核心表结构与字段语义 |
| 运维手册 | [OPERATIONS.md](./OPERATIONS.md) | 运维、值班人员 | 日常巡检、故障排查、备份恢复 |
| 常见问题 | [FAQ.md](./FAQ.md) | 所有人 | 快速排除高频问题 |

## 推荐阅读路径

1. 第一次接触项目：`QUICK_START` -> `ARCHITECTURE` -> `API_REFERENCE`
2. 准备上线：`CONFIGURATION` -> `DEPLOYMENT` -> `OPERATIONS`
3. 参与开发：`PROJECT_STRUCTURE` -> `DEVELOPMENT` -> `API_REFERENCE`

## 仓库级文档入口

- 中文主页：[../README.md](../README.md)
- 英文主页：[../README.en.md](../README.en.md)
- 第三方许可：[../THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md)

## 文档维护约定

1. 新增接口时，必须同步更新 `API_REFERENCE.md`。
2. 新增配置项时，必须同步更新 `CONFIGURATION.md`。
3. 变更核心流程（路由、同步、代理）时，必须同步更新 `ARCHITECTURE.md`。
4. 文档链接必须使用相对路径，确保仓库离线浏览可用。
