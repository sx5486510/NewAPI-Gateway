# Web 管理前端（React）

## 目录位置

- 前端源码目录：`web/`
- 页面入口：`web/src/pages`
- 组件目录：`web/src/components`
- 构建产物：`web/build`（由后端静态托管）

## 本地开发

```bash
# 在仓库根目录执行
npm --prefix web install
npm --prefix web start
```

默认开发端口是 `3001`，并通过 `package.json` 代理到后端 `3030`。

## 构建

```bash
# 在仓库根目录执行
npm --prefix web run build
```

如需指定 API 服务地址，请在构建前设置：

```bash
REACT_APP_SERVER=http://your.domain.com npm --prefix web run build
```

## 关联文档

- 开发指南：[../docs/DEVELOPMENT.md](../docs/DEVELOPMENT.md)
- 项目结构：[../docs/PROJECT_STRUCTURE.md](../docs/PROJECT_STRUCTURE.md)
- 文档中心：[../docs/README.md](../docs/README.md)
