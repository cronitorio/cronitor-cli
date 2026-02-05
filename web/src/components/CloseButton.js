import React from 'react';

export function CloseButton({ onClick }) {
  return (
    <button
      onClick={onClick}
      className="absolute top-0 right-8 bg-white dark:bg-gray-800 px-3 py-0 rounded-b-sm border border-t-0 border-gray-300 dark:border-gray-600 text-gray-400 hover:text-gray-500 dark:text-gray-400 dark:hover:text-gray-300 z-10 text-xl leading-none"
    >
      ×
    </button>
  );
} 