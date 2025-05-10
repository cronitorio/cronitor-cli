import React, { createContext, useContext, useReducer } from 'react';

const JobFormContext = createContext();

const initialState = {
  name: '',
  expression: '',
  command: '',
  crontab_filename: '',
  run_as_user: '',
  is_monitored: false,
  is_draft: true,
  timezone: 'UTC',
  errors: {},
  isDirty: false
};

function jobFormReducer(state, action) {
  switch (action.type) {
    case 'SET_FIELD':
      return {
        ...state,
        [action.field]: action.value,
        isDirty: true,
        errors: {
          ...state.errors,
          [action.field]: null
        }
      };
    case 'SET_ERRORS':
      return {
        ...state,
        errors: action.errors
      };
    case 'RESET_FORM':
      return {
        ...initialState
      };
    case 'INITIALIZE_FORM':
      return {
        ...action.initialState,
        isDirty: false,
        errors: {}
      };
    default:
      return state;
  }
}

export function JobFormProvider({ children, initialValues = {} }) {
  const [state, dispatch] = useReducer(jobFormReducer, {
    ...initialState,
    ...initialValues
  });

  const setField = (field, value) => {
    dispatch({ type: 'SET_FIELD', field, value });
  };

  const setErrors = (errors) => {
    dispatch({ type: 'SET_ERRORS', errors });
  };

  const resetForm = () => {
    dispatch({ type: 'RESET_FORM' });
  };

  const initializeForm = (values) => {
    dispatch({ type: 'INITIALIZE_FORM', initialState: values });
  };

  const validateForm = () => {
    const errors = {};
    if (!state.name) errors.name = 'Name is required';
    if (!state.expression) errors.expression = 'Schedule is required';
    if (!state.command) errors.command = 'Command is required';
    if (!state.crontab_filename) errors.crontab_filename = 'Location is required';
    if (!state.crontab_filename.startsWith('user') && !state.run_as_user) {
      errors.run_as_user = 'User is required for system crontabs';
    }
    setErrors(errors);
    return Object.keys(errors).length === 0;
  };

  const value = {
    ...state,
    setField,
    setErrors,
    resetForm,
    initializeForm,
    validateForm
  };

  return (
    <JobFormContext.Provider value={value}>
      {children}
    </JobFormContext.Provider>
  );
}

export function useJobForm() {
  const context = useContext(JobFormContext);
  if (!context) {
    throw new Error('useJobForm must be used within a JobFormProvider');
  }
  return context;
} 