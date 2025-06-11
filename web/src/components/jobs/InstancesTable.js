import React, { useState } from 'react';
import { ArrowPathIcon } from '@heroicons/react/24/outline';

export function InstancesTable({ 
  instances, 
  killingPids, 
  isKillingAll, 
  onKillInstance, 
  onKillAll,
  onRunNow 
}) {
  const [optimisticallyRemovedPids, setOptimisticallyRemovedPids] = useState(new Set());

  const handleKillInstance = (pid) => {
    // Call the original handler
    onKillInstance(pid);
    
    // Optimistically remove after a brief delay
    setTimeout(() => {
      setOptimisticallyRemovedPids(prev => new Set([...prev, pid]));
    }, 500); // 500ms delay to show the killing state briefly
  };

  // Filter out optimistically removed instances
  const visibleInstances = instances.filter(instance => !optimisticallyRemovedPids.has(instance.pid));
  return (
    <div className="mt-2">
      <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
        <thead>
          <tr>
            <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
              PID
            </th>
            <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
              Started
            </th>
            <th className="py-2 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
              Actions
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
          {visibleInstances.length > 0 ? (
            visibleInstances
              .sort((a, b) => new Date(a.started) - new Date(b.started))
              .map((instance) => (
              <tr key={instance.pid}>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  {instance.pid}
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  {instance.started}
                </td>
                <td className="py-2 text-right">
                  <button
                    onClick={() => handleKillInstance(instance.pid)}
                    disabled={killingPids.has(instance.pid)}
                    className={`text-xs text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300 inline-flex items-center space-x-1 ${
                      killingPids.has(instance.pid) ? 'opacity-30 cursor-not-allowed' : ''
                    }`}
                  >
                    {killingPids.has(instance.pid) ? (
                      <>
                        <ArrowPathIcon className="h-4 w-4 animate-spin" />
                        <span>Killing</span>
                      </>
                    ) : (
                      'Kill Now'
                    )}
                  </button>
                </td>
              </tr>
            ))
          ) : (
            <tr>
              <td colSpan="3" className="py-2 text-sm text-gray-500 dark:text-gray-400">
                None
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="mt-2 text-right">
        <button
          onClick={onRunNow}
          className="text-xs bg-blue-600 hover:bg-blue-700 text-white dark:bg-blue-500 dark:hover:bg-blue-600 px-3 py-1 rounded mr-1"
        >
          Run Now
        </button>
        {visibleInstances.length > 1 && (
          <button
            onClick={onKillAll}
            disabled={isKillingAll}
            className={`text-xs bg-red-600 hover:bg-red-700 text-white dark:bg-red-500 dark:hover:bg-red-600 px-3 py-1 ml-2 rounded ${
              isKillingAll ? 'opacity-30 cursor-not-allowed' : ''
            }`}
          >
            {isKillingAll ? (
              <>
                <ArrowPathIcon className="h-4 w-4 animate-spin inline-block mr-1" />
                <span>Killing</span>
              </>
            ) : (
              'Kill All'
            )}
          </button>
        )}
      </div>
    </div>
  );
} 