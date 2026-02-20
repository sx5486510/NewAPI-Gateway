export const reducer = (state, action) => {
  switch (action.type) {
    case 'set':
      return {
        ...state,
        theme: action.payload,
      };
    case 'toggle':
      return {
        ...state,
        theme: state.theme === 'dark' ? 'light' : 'dark',
      };
    default:
      return state;
  }
};

export const initialState = {
  theme: 'light',
};

