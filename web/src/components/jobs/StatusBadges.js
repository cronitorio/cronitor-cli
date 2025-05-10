import React from 'react';

export function StatusBadges({ 
  job, 
  instances, 
  showInstances, 
  onToggleInstances, 
  onToggleSuspended 
}) {
  return (
    <div className="absolute top-0 right-0 flex items-center">
      {!job.key ? (
        <div className="inline-flex items-center px-2.5 py-0.5 rounded-tr-lg rounded-bl-lg text-sm font-medium bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-300 z-20">
          DRAFT
        </div>
      ) : (
        <>
          {job.suspended ? (
            <div 
              onClick={onToggleSuspended}
              className="inline-flex items-center px-2.5 py-0.5 rounded-bl-lg text-sm font-medium bg-pink-100 dark:bg-pink-900/30 text-pink-700 dark:text-pink-300 cursor-pointer hover:bg-pink-200 dark:hover:bg-pink-800/30 border-r border-white dark:border-gray-600 z-20"
            >
              SUSPENDED
            </div>
          ) : (
            <div 
              onClick={onToggleSuspended}
              className="inline-flex items-center px-2.5 py-0.5 rounded-bl-lg text-sm font-medium bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300 cursor-pointer hover:bg-gray-300 dark:hover:bg-gray-600 border-r border-white dark:border-gray-600 z-20"
            >
              SCHEDULED
            </div>
          )}
          <button
            onClick={onToggleInstances}
            title={instances.length > 0 ? `${instances.length} instances of this job are running` : 'Job is not currently running'}
            className={`inline-flex items-center px-2.5 py-0.5 rounded-tr-lg text-sm font-medium ${
              instances.length > 0
                ? 'bg-blue-400 text-white hover:bg-blue-400/90'
                : 'bg-gray-200 dark:bg-gray-700 text-gray-600 dark:text-gray-300 hover:bg-gray-300 dark:hover:bg-gray-600'
            } z-20`}
          >
            {instances.length > 0 ? `RUNNING: ${instances.length}` : 'IDLE'}
          </button>
        </>
      )}
    </div>
  );
} 