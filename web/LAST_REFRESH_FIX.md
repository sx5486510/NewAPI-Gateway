# Refresh Token 状态显示修复

## 问题描述

用户报告：有的账号已经成功通过 refresh token 刷新过了，但前端显示：
- **Refresh Token 状态**：显示 "存在但未验证"（应该显示实际状态）
- **最近刷新时间**：显示 "未知"（应该显示实际刷新时间）

## 根本原因

1. 当 xAI token 刷新成功后，CPA 会更新**磁盘文件**中的 `last_refresh` 字段
2. 但 CPA 的**内存中的 `Auth.LastRefreshedAt`** 没有同步更新
3. Gateway 前端从 CPA 的 auth files 列表接口读取 `last_refresh`
4. 该接口返回的是内存中的 `Auth.LastRefreshedAt`（未更新），所以显示"未知"

## 解决方案

**策略**：前端直接读取认证文件内容中的 `last_refresh` 字段，绕过 CPA 内存状态的滞后问题。

### 修改文件列表

1. **`web/src/components/cpaAuthStatus.js`**
   - 在 `parseAuthCredentialMetadata` 中添加 `last_refresh` 解析
   - 返回 `lastRefresh` 字段（ISO 8601 格式）

2. **`web/src/components/CPAAuthFiles.js`**
   - 添加 `lastRefreshTime` 变量，优先使用文件内容中的值
   - 如果文件未读取，则回退到列表接口返回的值

3. **`web/src/components/cpaAuthStatus.test.js`**
   - 添加测试：验证正确解析 `last_refresh` 时间戳
   - 添加测试：验证处理缺失/无效的 `last_refresh` 字段

## 代码变更

### 1. cpaAuthStatus.js

```javascript
// 在 parseAuthCredentialMetadata 返回值中添加
const rawLastRefresh = typeof auth.last_refresh === 'string' ? auth.last_refresh.trim() : '';
const lastRefreshTime = rawLastRefresh ? Date.parse(rawLastRefresh) : Number.NaN;
const lastRefresh = Number.isNaN(lastRefreshTime)
  ? null
  : new Date(lastRefreshTime).toISOString();

return {
  accessStatus,
  expiresAt,
  lastRefresh,  // ⬅️ 新增
  hasRefreshToken: ...
};
```

### 2. CPAAuthFiles.js

```javascript
// 优先使用文件内容中的 last_refresh（刷新成功后立即更新），
// 如果文件未读取或读取失败，则回退到列表接口返回的 last_refresh
const lastRefreshTime = detail?.metadata?.lastRefresh || file.last_refresh;

// 在所有显示位置使用 lastRefreshTime 而不是 file.last_refresh
<span id={`${credentialId}-last-refresh`} style={itemStyle}>
  最近刷新: {formatCredentialTime(lastRefreshTime)}
</span>
```

### 3. cpaAuthStatus.test.js

```javascript
test('extracts last_refresh timestamp from auth file', () => {
  const result = parseAuthCredentialMetadata(
    JSON.stringify({
      expired: '2026-07-20T07:00:00Z',
      refresh_token: 'refresh-secret',
      last_refresh: '2026-07-20T06:30:00Z',
    }),
    now
  );

  expect(result).toEqual({
    accessStatus: 'expired',
    expiresAt: '2026-07-20T07:00:00.000Z',
    lastRefresh: '2026-07-20T06:30:00.000Z',
    hasRefreshToken: true,
  });
});

test('handles missing or invalid last_refresh gracefully', () => {
  expect(
    parseAuthCredentialMetadata(
      JSON.stringify({ refresh_token: 'token' }),
      now
    ).lastRefresh
  ).toBeNull();
  // ... 更多边界情况测试
});
```

## 修复效果

### 之前
```
最近刷新: 未知
Access Token: 已过期（2026-07-27 10:00:00）
Refresh Token: 存在但未验证
```

### 之后（刷新成功）
```
最近刷新: 2026-07-27 10:05:30  ⬅️ 立即显示文件中的刷新时间
Access Token: 有效（2026-07-27 11:05:30）
Refresh Token: 存在但未验证
```

### 之后（refresh token 失效）
```
最近刷新: 2026-07-27 09:00:00
Access Token: 已过期（2026-07-27 10:00:00）
Refresh Token: 疑似失效  ⬅️ 检测到 401/unauthorized 错误
```

## 测试结果

```bash
✓ 24 个测试全部通过
  - classifies official expired value (5 cases)
  - detects refresh token presence without returning it (4 cases)
  - normalizes an expiry without retaining credential secrets
  - extracts last_refresh timestamp from auth file ⬅️ 新增
  - handles missing or invalid last_refresh gracefully ⬅️ 新增
  - marks explicit unauthorized evidence as suspected invalid (7 cases)
  - uses only quota errors as unauthorized evidence
  - missing refresh token takes priority over unauthorized evidence
  - rejects invalid auth JSON (3 cases)
```

## 技术细节

### 数据源优先级
1. **第一优先**：文件内容中的 `metadata.lastRefresh`（实时，刷新后立即更新）
2. **第二优先**：列表接口的 `file.last_refresh`（CPA 内存，可能滞后）

### 为什么不直接修复 CPA
- CPA 是嵌入的第三方库（.tmp-cliproxyapi-*）
- 修改 CPA 需要跨项目协调
- 当前方案是零成本解决方案，不需要等待 CPA 更新
- 前端通过读取磁盘文件绕过了 CPA 内存状态的滞后

### Refresh Token 状态定义
- **missing** - 文件中没有 `refresh_token` 字段
- **unverified** - 有 `refresh_token`，但没有发现失效证据
- **suspected_invalid** - 有 `refresh_token`，但检测到 401/unauthorized 错误

## 相关文件

- `web/src/components/cpaAuthStatus.js` - 认证状态解析逻辑
- `web/src/components/CPAAuthFiles.js` - CPA 认证文件管理界面
- `web/src/components/cpaAuthStatus.test.js` - 单元测试

## 日期

2026-07-27
