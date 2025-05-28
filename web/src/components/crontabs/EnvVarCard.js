import React from 'react';

export function EnvVarCard({ line }) {
  return (
    <div className="bg-white dark:bg-gray-800 shadow rounded-lg p-4">
      <div className="space-y-2">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-lg font-medium text-gray-900 dark:text-white">Environment Variable</h3>
        </div>

        <div className="group relative">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead>
              <tr>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider w-[25%] min-w-[200px]">Variable Name</th>
                <th className="py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">Value</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              <tr>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <div className="font-mono">
                    {line.env_var_key || 'N/A'}
                  </div>
                </td>
                <td className="py-2 text-sm text-gray-900 dark:text-gray-100">
                  <div className="font-mono break-all">
                    {line.env_var_value || 'N/A'}
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
} 