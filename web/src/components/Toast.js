import React from 'react';
import { CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';

export function Toast({ message, onClose, type = 'error' }) {
  const bgColor = type === 'error' ? 'bg-red-500' : 'bg-green-500';
  return (
    <div className={`fixed bottom-4 left-4 ${bgColor} text-white px-4 py-2 rounded shadow-lg flex items-center space-x-2 z-50`}>
      {type === 'error' ? <XCircleIcon className="h-5 w-5" /> : <CheckCircleIcon className="h-5 w-5" />}
      <span className="text-white dark:text-gray-100">{message}</span>
      <button onClick={onClose} className="ml-2">
        ×
      </button>
    </div>
  );
} 