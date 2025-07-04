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
  const handleKeyDownEdit = (e) => {
    // For long commands in textarea, allow Shift+Enter for new lines
    if (e.key === 'Enter' && !e.shiftKey && editedCommand && editedCommand.length > 100) {
      e.preventDefault();
      onKeyDown(e);
    } else if (e.key === 'Enter' && !e.shiftKey) {
      onKeyDown(e);
    }
    // Let other keys pass through normally
    if (onKeyDown && e.key !== 'Enter') {
      onKeyDown(e);
    }
  };

  return (
    <>
      <div className="flex items-start gap-2">
        <div className="flex-1 min-w-0">
          <div className="flex items-start">
            {isEditing ? (
              // Use textarea for long commands, input for short ones
              editedCommand && editedCommand.length > 100 ? (
                <textarea
                  value={editedCommand}
                  onChange={(e) => onCommandChange(e.target.value)}
                  onKeyDown={handleKeyDownEdit}
                  onBlur={onEditEnd}
                  className={isNew 
                    ? "w-full text-sm text-gray-800 dark:text-gray-100 font-mono rounded-md border-gray-400 dark:border-gray-600 shadow-sm focus:border-blue-500 focus:ring-blue-500 dark:bg-gray-700 dark:text-white px-3 py-2 bg-gray-200 resize-y min-h-[2.5rem]" 
                    : "w-full text-sm text-gray-800 dark:text-gray-100 font-mono bg-transparent border border-gray-400 dark:border-gray-600 focus:outline-none focus:border-blue-500 rounded px-2 py-1 resize-y min-h-[2.5rem]"
                  }
                  placeholder={isNew ? "/path/to/command --with --arguments" : ""}
                  rows={Math.min(Math.max(2, Math.ceil(editedCommand.length / 80)), 6)}
                />
              ) : (
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
              )
            ) :
              <div 
                className="text-sm text-gray-900 dark:text-gray-100 font-mono break-all overflow-wrap-anywhere whitespace-pre-wrap"
                title={job.command}
              >
                {job.command}
              </div>
            }
            {!isNew && !readOnly && (
              <button
                onClick={onEditStart}
                className="ml-2 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity"
              >
                <PencilIcon className="h-4 w-4 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200" />
              </button>
            )}
          </div>
        </div>
      </div>
    </>
  );
} 