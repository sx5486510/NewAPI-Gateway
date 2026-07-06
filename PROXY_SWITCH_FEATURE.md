# 代理开关功能说明

## 功能概述
在系统设置页面添加了"启用代理"开关，允许管理员动态控制是否使用配置的代理服务器。

## 实现细节

### 后端修改

#### 1. `common/constants.go`
- 添加全局变量 `ProxyEnabled = false`，默认禁用代理

#### 2. `common/proxy.go`
- 修改 `ProxyFromSettings()` 函数，首先检查 `ProxyEnabled` 开关
- 只有当 `ProxyEnabled` 为 `true` 时，才使用配置的代理地址
- 否则回退到环境变量代理（`http.ProxyFromEnvironment`）

#### 3. `model/option.go`
- 在 `InitOptionMap()` 中添加 `ProxyEnabled` 选项初始化
- 在 `updateOptionMap()` 的 `Enabled` 处理分支中添加 `ProxyEnabled` 的更新逻辑
- 确保数据库中的 `ProxyEnabled` 配置能正确同步到内存变量

### 前端修改

#### `web/src/components/SystemSetting.js`
1. **State 管理**
   - 在 `inputs` 初始状态中添加 `ProxyEnabled: ''`
   
2. **Checkbox 处理**
   - 在 `updateOption()` 的 switch-case 中添加 `ProxyEnabled` 处理
   - 支持切换 `true`/`false` 状态

3. **UI 布局**
   - 在代理地址输入框上方添加"启用代理"复选框
   - 当代理未启用时，禁用代理地址输入框
   - 当代理未启用时，禁用"保存代理设置"和"测试代理"按钮
   - 通过 `disabled` 属性提供视觉反馈

## 使用说明

### 管理员操作流程
1. 登录管理后台，进入"系统设置"页面
2. 找到"通用设置"部分的代理配置区域
3. 勾选"启用代理"复选框以启用代理功能
4. 输入代理地址（例如：`http://127.0.0.1:7890`）
5. 点击"保存代理设置"保存配置
6. 可选：点击"测试代理"验证代理连接性
7. 取消勾选"启用代理"可立即禁用代理，无需删除代理地址

### 配置保留
- 即使取消勾选"启用代理"，已配置的代理地址仍会保留在数据库中
- 下次勾选"启用代理"时，无需重新输入代理地址

## 技术优势

1. **无缝切换**：可在不重启服务的情况下启用/禁用代理
2. **配置保留**：禁用代理不会清空配置，方便临时切换
3. **用户友好**：通过 UI 禁用状态直观提示用户当前状态
4. **向后兼容**：保留对环境变量代理的支持（`HTTP_PROXY`、`HTTPS_PROXY`）

## 测试建议

1. 验证开关启用/禁用后代理确实生效/失效
2. 测试禁用状态下无法编辑代理地址和保存
3. 测试禁用后重新启用，配置仍然保留
4. 测试代理测试功能在各状态下的正确性

## 相关文件

### 后端
- `common/constants.go` - 全局变量定义
- `common/proxy.go` - 代理逻辑实现
- `model/option.go` - 配置持久化

### 前端
- `web/src/components/SystemSetting.js` - 设置页面 UI
- `web/src/components/ui/Input.js` - Input 组件（已支持 disabled）

## 构建状态
✅ 前端构建通过（main.js +382 B）  
✅ 后端编译通过（无错误）  
✅ Go vet 检查通过
