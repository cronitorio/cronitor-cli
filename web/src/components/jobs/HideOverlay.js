import React from 'react';
import { CloseButton } from '../CloseButton';

export function HideOverlay({
  job,
  onClose,
  onHideJob
}) {
  const handleHideClick = () => {
    onHideJob();
  };
  
  return (
    <div className="absolute inset-0 bg-black bg-opacity-50 rounded-lg flex items-center justify-center z-10" style={{ top: '-8px' }}>
      <div className="bg-white dark:bg-gray-800 p-8 rounded-lg shadow-xl max-w-xl w-full mx-4 relative">
        <CloseButton onClick={onClose} />
        <h3 
          className="text-lg font-medium text-gray-900 dark:text-white mb-2 cursor-pointer hover:text-gray-600 dark:hover:text-gray-300"
          onClick={onClose}
          title="Click to close"
        >
          Hide Job
        </h3>
        <p className="text-gray-600 dark:text-gray-300 mb-4">
          If you hide this job, it will not be shown on the dashboard, but it will still run like normal.
        </p>
        <div className="flex justify-end space-x-4">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-700 border border-gray-300 dark:border-gray-600 rounded-md hover:bg-gray-50 dark:hover:bg-gray-600"
          >
            Cancel
          </button>
          <button
            onClick={handleHideClick}
            className="px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-md"
          >
            Hide Job
          </button>
        </div>
      </div>
    </div>
  );
} 