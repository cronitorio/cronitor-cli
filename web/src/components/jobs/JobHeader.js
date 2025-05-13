import React from 'react';
import { PencilIcon } from '@heroicons/react/24/outline';

export function JobHeader({ 
  job, 
  isEditing, 
  editedName, 
  onNameChange, 
  onEditStart, 
  onEditEnd, 
  onKeyDown,
  isNew = false
}) {
  return (
    <div className="group relative">
      <div className="flex items-center">
        {isEditing ? (
          <input
            type="text"
            value={editedName}
            onChange={(e) => onNameChange(e.target.value)}
            onKeyDown={onKeyDown}
            onBlur={onEditEnd}
            className={`w-[94%] ${isNew 
              ? "text-lg font-medium text-gray-900 dark:text-white rounded-md border-gray-300 dark:border-gray-600 shadow-sm focus:border-blue-500 focus:ring-blue-500 dark:bg-gray-700 dark:text-white px-3 py-2"
              : "text-lg font-medium text-gray-900 dark:text-white bg-transparent border-b border-gray-300 dark:border-gray-600 focus:outline-none"
            }`}
            placeholder={isNew ? "Job Name" : ""}
          />
        ) : (
          <div className="text-lg font-medium text-gray-900 dark:text-white truncate">
            {job.name || job.default_name}
          </div>
        )}
        {!isNew && (
          <button
            onClick={onEditStart}
            className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
          >
            <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
          </button>
        )}
      </div>
    </div>
  );
} 