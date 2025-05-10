import React from 'react';

export function LocationSection({
  job,
  isNew,
  cronFiles,
  users,
  selectedLocation,
  selectedUser,
  isUserCrontab,
  onLocationChange,
  onUserChange
}) {
  return (
    <div className="">
      {isNew ? (
        <>
          <select
            value={selectedLocation}
            onChange={onLocationChange}
            className="w-full bg-gray-50 dark:bg-gray-700/50 border-0 focus:ring-1 focus:ring-blue-500/50 rounded-md px-3 py-2"
          >
            <option value="">Select a location</option>
            {cronFiles.map((file) => (
              <option key={file.filename} value={file.filename}>
                {file.display_name}
              </option>
            ))}
            <option value="/etc/cron.d">/etc/cron.d (New Crontab)</option>
          </select>
          {selectedLocation && !isUserCrontab && (
            <select
              value={selectedUser}
              onChange={onUserChange}
              className="w-full mt-2 bg-gray-50 dark:bg-gray-700/50 border-0 focus:ring-1 focus:ring-blue-500/50 rounded-md px-3 py-2"
            >
              <option value="">Select a user</option>
              {users.map((user) => (
                <option key={user} value={user}>{user}</option>
              ))}
            </select>
          )}
        </>
      ) : (
        <div>
          {job.crontab_filename.startsWith('user') ? 'User' + job.crontab_filename.slice(4) : job.crontab_filename} L{job.line_number}
        </div>
      )}
    </div>
  );
} 