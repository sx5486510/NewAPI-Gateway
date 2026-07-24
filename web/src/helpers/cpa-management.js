export const requireCPASuccess = (response) => {
  if (response?.data?.success === false) {
    const error = new Error(response.data.message || 'CPA 管理请求失败');
    const code =
      typeof response.data.code === 'string' ? response.data.code.trim() : '';
    if (code) {
      error.code = code;
    }
    // Gateway 在刷新 xAI 凭证失败时返回 HTTP 502 + code，
    // 映射为 401 以便「额度返回 401」筛选与 refresh 可疑状态能命中。
    if (code === 'auth_token_refresh_failed') {
      error.status = 401;
    }
    throw error;
  }
  return response;
};
