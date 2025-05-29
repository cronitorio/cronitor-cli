import React from 'react';
import { PencilIcon } from '@heroicons/react/24/outline';

export function CommandSection({
  job,
  isEditing,
  editedCommand,
  onCommandChange,
  onEditStart,
  onEditEnd,
  onKeyDown,
  onShowConsole,
  isNew = false,
  readOnly = false
}) {
  return (
    <>
      <div className="flex items-center justify-between">
        <div className="flex-1">
          <div className="flex items-center">
            {isEditing ? (
              <input
                type="text"
                value={editedCommand}
                onChange={(e) => onCommandChange(e.target.value)}
                onKeyDown={onKeyDown}
                onBlur={onEditEnd}
                className={isNew 
                  ? "w-full text-sm text-gray-800 dark:text-gray-100 font-mono rounded-md border-gray-400 dark:border-gray-600 shadow-sm focus:border-blue-500 focus:ring-blue-500 dark:bg-gray-700 dark:text-white px-3 py-2 bg-gray-200" 
                  : "w-full text-sm text-gray-800 dark:text-gray-100 font-mono bg-transparent border-b border-gray-400 dark:border-gray-600 focus:outline-none"
                }
                placeholder={isNew ? "/path/to/command" : ""}
              />
            ) : (
              <div className="text-sm text-gray-900 dark:text-gray-100 font-mono truncate">
                {job.command}
              </div>
            )}
            {!isNew && !readOnly && (
              <button
                onClick={onEditStart}
                className="ml-2 opacity-0 group-hover:opacity-100 transition-opacity"
              >
                <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
              </button>
            )}
          </div>
        </div>
        {!readOnly && (
          <button
            onClick={onShowConsole}
            className="ml-4 text-sm text-gray-900 hover:text-black dark:text-gray-300 dark:hover:text-white"
          >
            Console
          </button>
        )}
      </div>
    </>
  );
} 