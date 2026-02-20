import { showError } from './utils';
import axios from 'axios';

export const API = axios.create({
  baseURL: process.env.REACT_APP_SERVER ? process.env.REACT_APP_SERVER : '',
});

API.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    let errorMessage = '未知错误';
    if (error.response && error.response.data && error.response.data.message) {
      errorMessage = error.response.data.message;
    } else if (error.message) {
      errorMessage = error.message;
    }
    showError(errorMessage);
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
