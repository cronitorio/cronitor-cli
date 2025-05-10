import React from 'react';
import { CloseButton } from '../CloseButton';

export function DeleteConfirmation({
  onClose,
  onDelete,
  deleteConfirmation,
  onDeleteConfirmationChange
}) {
  return (
    <div className="absolute inset-0 bg-black bg-opacity-50 rounded-lg flex items-center justify-center z-10">
      <div className="bg-white dark:bg-gray-800 p-8 rounded-lg shadow-xl max-w-lg w-full mx-4 relative">
        <CloseButton onClick={onClose} />
        <h3 className="text-lg font-medium text-gray-900 dark:text-white mb-2">
          Delete Job
        </h3>
        <p className="text-gray-600 dark:text-gray-300 mb-4">
          Type DELETE to confirm. This cannot be undone.
        </p>
        <input
          type="text"
          placeholder="DELETE"
          value={deleteConfirmation}
          onChange={(e) => onDeleteConfirmationChange(e.target.value)}
          className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-md focus:outline-none focus:ring-red-500 focus:border-red-500"
        />
        <div className="mt-4 flex justify-end">
          <button
            onClick={onDelete}
            disabled={deleteConfirmation.toLowerCase() !== 'delete'}
            className={`px-4 py-2 text-sm font-medium text-white rounded-md ${
              deleteConfirmation.toLowerCase() === 'delete'
                ? 'bg-red-600 hover:bg-red-700'
                : 'bg-gray-400 cursor-not-allowed'
            }`}
          >
            Delete Job
          </button>
        </div>
      </div>
    </div>
  );
} 