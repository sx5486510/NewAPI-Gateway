export const requireCPASuccess = (response) => {
  if (response?.data?.success === false) {
    throw new Error(response.data.message || 'CPA 管理请求失败');
  }
  return response;
};
