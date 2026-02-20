import axios from 'axios';

export const API = axios.create({
  baseURL: process.env.REACT_APP_SERVER ? process.env.REACT_APP_SERVER : '',
});

API.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    if (error?.response?.status === 401) {
      localStorage.removeItem('user');
      if (!window.location.pathname.startsWith('/login')) {
        window.location.href = '/login?expired=true';
      }
      return Promise.resolve({
        data: {
          success: false,
          message: '未登录或登录已过期，请重新登录',
        }
      });
    }

    let errorMessage = '未知错误';
    if (error?.response?.status === 429) {
      errorMessage = '请求次数过多，请稍后再试';
    } else if (error.response && error.response.data && error.response.data.message) {
      errorMessage = error.response.data.message;
    } else if (error.message) {
      errorMessage = error.message;
    }
    // Return a fake response so that `const res = await API.get(...)` doesn't get undefined
    // and `res.data` won't crash the frontend.
    return Promise.resolve({
      data: {
        success: false,
        message: errorMessage,
      }
    });
  }
);
