import React, { useState, useEffect, useRef } from 'react';
import useSWR from 'swr';
import { NewCrontabOverlay } from './jobs/NewCrontabOverlay';
import { Toast } from './Toast';
import { JobCard } from './jobs/JobCard';
import { CommentCard } from './crontabs/CommentCard';
import { EnvVarCard } from './crontabs/EnvVarCard';

const fetcher = async url => {
  try {
    const res = await fetch(url);
    if (!res.ok) {
      const errorText = await res.text();
      throw new Error(`Server error: ${res.status} ${res.statusText}${errorText ? ` - ${errorText}` : ''}`);
    }
    return res.json();
  } catch (error) {
    if (error.name === 'TypeError' && error.message === 'Failed to fetch') {
      throw new Error(`Unable to connect to the dash server. Check that it's running and try again.`);
    }
    throw error;
  }
};

export default function Crontabs() {
  const [enableAutoRefresh, setEnableAutoRefresh] = useState(false);
  const [revalidationKey, setRevalidationKey] = useState(0);
  const [isEditing, setIsEditing] = useState(false);
  const [editedContent, setEditedContent] = useState('');
  const textareaRef = useRef(null);
  
  // Enable auto-refresh after initial load completes
  useEffect(() => {
    const timer = setTimeout(() => {
      setEnableAutoRefresh(true);
    }, 2000); // Wait 2 seconds before enabling auto-refresh
    
    return () => clearTimeout(timer);
  }, []);
  
  // Load settings first (non-critical, no refresh)
  const { data: settings } = useSWR('/api/settings', fetcher, {
    revalidateOnFocus: false,
    refreshInterval: 0
  });
  
  // Load crontabs with controlled refresh
  const { data: crontabs, error, mutate } = useSWR(
    `/api/crontabs?key=${revalidationKey}`,
    fetcher,
    {
      refreshInterval: enableAutoRefresh ? 10000 : 0, // Only auto-refresh after initial load
      revalidateOnFocus: true, // Enable revalidation on focus
      dedupingInterval: 2000 // Prevent duplicate requests within 2s
    }
  );
  
  // Only load jobs after crontabs are loaded
  const { data: jobs, mutate: jobsMutate } = useSWR(
    crontabs ? `/api/jobs?key=${revalidationKey}` : null, 
    fetcher, 
    {
      refreshInterval: enableAutoRefresh ? 10000 : 0, // Only auto-refresh after initial load
      revalidateOnFocus: true,
      dedupingInterval: 2000
    }
  );
  
  const [selectedCrontab, setSelectedCrontab] = useState(null);
  const [selectedLine, setSelectedLine] = useState(null);
  const [showNewCrontab, setShowNewCrontab] = useState(false);
  const [isToastVisible, setIsToastVisible] = useState(false);
  const [toastMessage, setToastMessage] = useState('');
  const [toastType, setToastType] = useState('error');
  const [isInitialLoad, setIsInitialLoad] = useState(true);
  
  const [newCrontabForm, setNewCrontabForm] = useState({
    filename: '',
    timezone: '',
    comments: ''
  });

  // Update timezone when settings are loaded
  useEffect(() => {
    if (settings?.timezone) {
      setNewCrontabForm(prev => ({
        ...prev,
        timezone: settings.timezone
      }));
    }
  }, [settings]);

  // Select first crontab when data loads
  useEffect(() => {
    if (crontabs && crontabs.length > 0 && !selectedCrontab) {
      setSelectedCrontab(crontabs[0]);
      setIsInitialLoad(false);
    }
  }, [crontabs, selectedCrontab]);

  // Update selectedLine when selectedCrontab changes to keep data in sync
  useEffect(() => {
    if (selectedCrontab && selectedLine) {
      // Find the line index in the current crontab
      const currentIndex = selectedCrontab.lines.findIndex(line => line === selectedLine);
      if (currentIndex === -1) {
        // The selectedLine is from an old crontab, find the corresponding line by index
        const originalIndex = selectedLine.line_number;
        if (originalIndex >= 0 && originalIndex < selectedCrontab.lines.length) {
          const updatedLine = selectedCrontab.lines[originalIndex];
          if (updatedLine && updatedLine !== selectedLine) {
            setSelectedLine(updatedLine);
          }
        }
      }
    }
  }, [selectedCrontab, selectedLine]);

  // Debug selected line
  useEffect(() => {
    if (selectedLine) {
      console.log('Selected line full object:', selectedLine);
      console.log('Line type checks:', {
        is_job: selectedLine.is_job,
        is_job_type: typeof selectedLine.is_job,
        is_job_truthy: !!selectedLine.is_job,
        is_comment: selectedLine.is_comment,
        is_comment_type: typeof selectedLine.is_comment,
        is_env_var: selectedLine.is_env_var,
        is_env_var_type: typeof selectedLine.is_env_var,
        has_line_text: !!selectedLine.line_text,
        line_text_preview: selectedLine.line_text?.substring(0, 50)
      });
    }
  }, [selectedLine]);

  const showToast = (message, type = 'error') => {
    setToastMessage(message);
    setToastType(type);
    setIsToastVisible(true);
    setTimeout(() => setIsToastVisible(false), type === 'error' ? 6000 : 3000);
  };

  const handleCreateCrontab = async () => {
    try {
      if (!newCrontabForm.timezone && settings?.timezone) {
        setNewCrontabForm(prev => ({
          ...prev,
          timezone: settings.timezone
        }));
      }
      
      const response = await fetch('/api/crontabs', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          filename: newCrontabForm.filename,
          TimezoneLocationName: {
            Name: newCrontabForm.timezone
          },
          comments: newCrontabForm.comments
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to create crontab');
      }

      const crontab = await response.json();
      
      setShowNewCrontab(false);
      showToast(
        response.status === 201 ? 'Crontab Created' : 'Crontab Already Exists',
        'success'
      );
      
      // Refresh crontabs list
      mutate();
      
      // Select the new crontab
      setSelectedCrontab(crontab);
      
      // Reset form
      setNewCrontabForm({
        filename: '',
        timezone: settings?.timezone || '',
        comments: ''
      });
    } catch (error) {
      console.error('Error creating crontab:', error);
      showToast('Failed to create crontab: ' + error.message);
    }
  };

  const handleLineClick = (lineIndex) => {
    if (selectedCrontab && selectedCrontab.lines) {
      setSelectedLine(selectedCrontab.lines[lineIndex]);
    }
  };

  const formatCrontabContent = () => {
    if (!selectedCrontab || !selectedCrontab.lines) return [];
    
    const formattedLines = [];
    let lineNumber = 1;
    
    selectedCrontab.lines.forEach((line) => {
      // If this is a job line with a name, add the Name comment first
      if (line.is_job && line.name) {
        formattedLines.push({
          lineNumber: lineNumber++,
          text: `# Name: ${line.name}`,
          originalLine: line,
          isNameComment: true
        });
      }
      
      // Add the actual line
      let displayText = line.line_text || '';
      
      // If this is a comment line and doesn't already start with #, add it
      if (line.is_comment && displayText && !displayText.startsWith('#')) {
        displayText = `# ${displayText}`;
      }
      
      formattedLines.push({
        lineNumber: lineNumber++,
        text: displayText,
        originalLine: line,
        isNameComment: false
      });
    });
    
    return formattedLines;
  };

  // Get or create a job object for the selected line
  const getJobForLine = () => {
    if (!selectedLine || !selectedLine.is_job) return null;
    
    // If the line has a job object from the server, use it
    if (selectedLine.job) {
      // Add any missing fields that might be needed
      const job = { ...selectedLine.job };
      
      // Add instances from the jobs list if available
      if (jobs && jobs.length > 0 && selectedLine.key) {
        const matchedJob = jobs.find(j => j.key === selectedLine.key);
        if (matchedJob && matchedJob.instances) {
          job.instances = matchedJob.instances;
        }
      }
      
      return job;
    }
    
    // Fallback for older server versions that don't include job in the response
    // This code can be removed once all servers are updated
    if (jobs && jobs.length > 0) {
      // Try to match by key
      if (selectedLine.key) {
        const job = jobs.find(j => j.key === selectedLine.key);
        if (job) return { ...job, line_number: selectedLine.line_number };
      }
      
      // Try to match by content
      const matchedJob = jobs.find(j => 
        j.crontab_filename === selectedLine.crontab_filename &&
        j.expression === selectedLine.cron_expression &&
        j.command === selectedLine.command_to_run
      );
      if (matchedJob) return { ...matchedJob, line_number: selectedLine.line_number };
    }
    
    // If no match found, create a job object from the crontab line
    // Important: If we have a code but the job wasn't found in the jobs list,
    // it means the monitor might not exist, so we shouldn't assume it's monitored
    return {
      key: selectedLine.key || `crontab-line-${selectedLine.line_number}`,
      code: selectedLine.code,
      name: selectedLine.name || selectedLine.default_name || '',
      command: selectedLine.command_to_run || '',
      expression: selectedLine.cron_expression || '',
      crontab_filename: selectedLine.crontab_filename || selectedCrontab?.filename || '',
      crontab_display_name: selectedCrontab?.display_name || selectedCrontab?.filename || '',
      line_number: selectedLine.line_number,
      run_as_user: selectedLine.run_as || '',
      timezone: selectedLine.timezone || selectedCrontab?.timezone || 'UTC',
      monitored: false, // Default to false since we couldn't find it in the jobs list
      suspended: false,
      instances: [],
      is_draft: false
    };
  };

  const handleEditStart = (displayedLineIndex) => {
    if (!selectedCrontab) return;
    
    // Get all lines including Name comments (same as what's displayed)
    const lines = selectedCrontab.lines.map(line => {
      if (line.is_job && line.name) {
        // For job lines with names, add the Name comment first, then the job line
        const jobLineText = line.is_comment ? `# ${line.line_text}` : line.line_text;
        return [`# Name: ${line.name}`, jobLineText];
      } else {
        // For other lines, add # prefix if it's a comment line and doesn't already have it
        let lineText = line.line_text;
        if (line.is_comment && lineText && !lineText.startsWith('#')) {
          lineText = `# ${lineText}`;
        }
        return [lineText];
      }
    }).flat();
    
    const content = lines.join('\n');
    
    // Calculate the position to place the cursor at the end of the clicked line
    // Use the displayedLineIndex which matches the lines array
    const linesBeforeCursor = lines.slice(0, displayedLineIndex + 1);
    const cursorPosition = linesBeforeCursor.join('\n').length;
    
    setEditedContent(content);
    setIsEditing(true);
    setSelectedLine(null); // Clear selected line when editing starts
    
    // Focus the textarea and set cursor position after a brief delay
    setTimeout(() => {
      if (textareaRef.current) {
        textareaRef.current.focus();
        textareaRef.current.setSelectionRange(cursorPosition, cursorPosition);
      }
    }, 0);
  };

  const handleDiscard = () => {
    setIsEditing(false);
    setEditedContent('');
  };

  const handleSubmit = async () => {
    try {
      // Split content into lines and process them
      const lines = editedContent.split('\n').map(line => line.trim());
      const processedLines = [];
      let currentName = null;

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        
        // Check if this is a Name comment (handles both # Name: and #Name: formats)
        const nameMatch = line.match(/^#\s*Name:\s*(.+)$/i);
        if (nameMatch) {
          currentName = nameMatch[1].trim();
          // Don't add the Name comment to processed lines
          continue;
        }

        // Check if this is a job line (either active or suspended)
        const isJobLine = line && (
          !line.startsWith('#') || // Active job line
          (line.startsWith('#') && line.trim().length > 1 && !line.match(/^#\s*Name:/i)) // Suspended job line (but not a Name comment)
        );

        if (isJobLine) {
          processedLines.push({
            line_text: line,
            name: currentName || undefined
          });
          currentName = null; // Reset the name after associating it
        } else {
          // Regular comment, empty line, or environment variable
          processedLines.push({ line_text: line });
        }
      }
      
      const response = await fetch(`/api/crontabs/${selectedCrontab.filename}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ lines: processedLines }),
      });

      if (!response.ok) {
        throw new Error('Failed to update crontab');
      }

      const updatedCrontab = await response.json();
      
      // Update local state immediately
      setSelectedCrontab(updatedCrontab);
      setIsEditing(false);
      setEditedContent('');
      
      // Update the SWR cache with the new crontab data
      mutate(
        (currentData) => {
          return currentData.map(crontab => 
            crontab.filename === updatedCrontab.filename ? updatedCrontab : crontab
          );
        },
        { revalidate: false }
      );
      
      // Force revalidation of both crontabs and jobs data
      forceRevalidation();
      mutate();
      jobsMutate();
      
      showToast('Crontab updated successfully', 'success');
    } catch (error) {
      console.error('Error updating crontab:', error);
      showToast('Failed to update crontab: ' + error.message);
    }
  };

  const forceRevalidation = () => {
    setRevalidationKey(prev => prev + 1);
  };

  if (error) {
    return (
      <div className="p-4 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800">
        <div className="flex">
          <div className="flex-shrink-0">
            <svg className="h-5 w-5 text-red-400" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
            </svg>
          </div>
          <div className="ml-3 flex-1">
            <h3 className="text-sm font-medium text-red-800 dark:text-red-200">Failed to load crontabs</h3>
            <div className="mt-2 text-sm text-red-700 dark:text-red-300">
              <pre className="whitespace-pre-wrap break-words">{error.message}</pre>
            </div>
            {error.message.includes('Unable to connect') && (
              <div className="mt-2 text-sm text-red-700 dark:text-red-300">
                <p>Possible causes:</p>
                <ul className="list-disc list-inside mt-1">
                  <li>The dash server is not running</li>
                  <li>Your IP is not whitelisted</li>
                  <li>A VPN connection is required</li>
                  <li>Server is restarting</li>
                </ul>
              </div>
            )}
            <div className="mt-4">
              <button
                onClick={() => mutate()}
                className="inline-flex items-center px-4 py-2 border border-gray-300 dark:border-gray-600 text-sm font-medium rounded-md shadow-sm text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
              >
                Reload
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (!crontabs || isInitialLoad) {
    return <div className="text-gray-600 dark:text-gray-300">Loading...</div>;
  }

  const jobForLine = getJobForLine();

  return (
    <div className="h-full flex flex-col">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Crontabs</h1>
        {settings?.os !== 'darwin' && !settings?.safe_mode && (
          <button
            onClick={() => setShowNewCrontab(true)}
            className="px-4 py-2.5 bg-blue-600 text-white rounded hover:bg-blue-700 font-medium"
          >
            Add Crontab
          </button>
        )}
      </div>

      <div className="mb-4 w-1/2">
        <select
          value={selectedCrontab?.filename || ''}
          onChange={(e) => {
            const crontab = crontabs.find(c => c.filename === e.target.value);
            setSelectedCrontab(crontab);
            setSelectedLine(null);
          }}
          className="block w-full pl-3 pr-10 py-2 text-base border-gray-300 dark:border-gray-600 focus:outline-none focus:ring-blue-500 focus:border-blue-500 rounded-md bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 appearance-none bg-[url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20fill%3D%22none%22%20viewBox%3D%220%200%2020%2020%22%3E%3Cpath%20stroke%3D%22%236B7280%22%20stroke-linecap%3D%22round%22%20stroke-linejoin%3D%22round%22%20stroke-width%3D%221.5%22%20d%3D%22M6%208l4%204%204-4%22%2F%3E%3C%2Fsvg%3E')] bg-[length:1.5em_1.5em] bg-[right_0.5rem_center] bg-no-repeat"
        >
          {crontabs.map((crontab) => (
            <option key={crontab.filename} value={crontab.filename}>
              {(crontab.display_name || crontab.filename).replace(/^user /, 'User ')}
            </option>
          ))}
        </select>
      </div>

      <div className="flex-1 flex flex-col gap-4 min-h-0">
        {/* Main content area - full width */}
        <div className="flex-1 bg-white dark:bg-gray-800 rounded-lg shadow overflow-hidden">
          <div className="h-full rounded-lg">
            <div className="h-full rounded-lg overflow-hidden" style={{ backgroundColor: 'rgba(0, 0, 0, 0.9)' }}>
              <div className="h-full flex">
                {/* Line numbers column */}
                <div className="flex-shrink-0 py-4 pl-4 pr-2 text-gray-400 dark:text-gray-500 font-mono text-sm select-none" style={{ lineHeight: '1.5' }}>
                  {isEditing ? (
                    editedContent.split('\n').map((_, index) => (
                      <div key={index}>{String(index + 1).padStart(3, ' ')}</div>
                    ))
                  ) : (
                    formatCrontabContent().map((line, index) => (
                      <div key={index}>{String(line.lineNumber).padStart(3, ' ')}</div>
                    ))
                  )}
                </div>
                {/* Separator */}
                <div className="flex-shrink-0 py-4 text-gray-300 dark:text-gray-600 font-mono text-sm select-none" style={{ lineHeight: '1.5' }}>
                  {isEditing ? (
                    editedContent.split('\n').map((_, index) => (
                      <div key={index}>│</div>
                    ))
                  ) : (
                    formatCrontabContent().map((_, index) => (
                      <div key={index}>│</div>
                    ))
                  )}
                </div>
                {/* Content area */}
                <div className="flex-1 overflow-auto">
                  {isEditing ? (
                    <textarea
                      ref={textareaRef}
                      value={editedContent}
                      onChange={(e) => setEditedContent(e.target.value)}
                      className="w-full h-full py-4 px-3 font-mono text-sm text-gray-100 bg-transparent border-none focus:ring-0 resize-none"
                      style={{ lineHeight: '1.5' }}
                    />
                  ) : (
                    <div 
                      className="py-4 px-3 font-mono text-sm text-gray-100 cursor-text min-h-full"
                      style={{ lineHeight: '1.5' }}
                      tabIndex={0}
                      onClick={() => {
                        // If crontab is empty, start editing immediately
                        if (!settings?.safe_mode && (!selectedCrontab || !selectedCrontab.lines || selectedCrontab.lines.length === 0)) {
                          setEditedContent('');
                          setIsEditing(true);
                          setSelectedLine(null);
                          
                          // Focus the textarea after a brief delay
                          setTimeout(() => {
                            if (textareaRef.current) {
                              textareaRef.current.focus();
                              textareaRef.current.setSelectionRange(0, 0);
                            }
                          }, 0);
                        }
                      }}
                      onKeyDown={(e) => {
                        // Handle keyboard navigation
                        const formattedLines = formatCrontabContent();
                        const currentIndex = selectedLine ? formattedLines.findIndex(fl => fl.originalLine === selectedLine) : -1;
                        let newIndex = currentIndex;
                        
                        if (e.key === 'ArrowUp' && currentIndex > 0) {
                          newIndex = currentIndex - 1;
                          // Skip name comment lines
                          while (newIndex >= 0 && formattedLines[newIndex].isNameComment) {
                            newIndex--;
                          }
                          e.preventDefault();
                        } else if (e.key === 'ArrowDown' && currentIndex < formattedLines.length - 1) {
                          newIndex = currentIndex + 1;
                          // Skip name comment lines
                          while (newIndex < formattedLines.length && formattedLines[newIndex].isNameComment) {
                            newIndex++;
                          }
                          e.preventDefault();
                        }
                        
                        if (newIndex !== currentIndex && newIndex >= 0 && newIndex < formattedLines.length && !formattedLines[newIndex].isNameComment) {
                          setSelectedLine(formattedLines[newIndex].originalLine);
                        }
                      }}
                    >
                      {formatCrontabContent().length === 0 ? (
                        <div className="text-gray-500 italic">
                          {settings?.safe_mode ? 'Empty crontab' : 'Empty crontab - click to edit'}
                        </div>
                      ) : (
                        formatCrontabContent().map((line, index) => (
                          <div
                            key={index}
                            onClick={() => line.originalLine && handleLineClick(selectedCrontab.lines.indexOf(line.originalLine))}
                            onDoubleClick={() => !settings?.safe_mode && handleEditStart(index)}
                            className={`pr-4 ${
                              !line.isNameComment ? 'hover:bg-gray-900 cursor-pointer' : 'hover:bg-gray-900 cursor-pointer'
                            } ${
                              line.originalLine === selectedLine ? 'bg-blue-900/30' : ''
                            } ${
                              line.isNameComment || (line.originalLine && line.originalLine.is_comment) ? 'text-gray-400' : ''
                            }`}
                          >
                            {line.text || '\u00A0'}
                          </div>
                        ))
                      )}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Edit controls or detail card */}
        {isEditing ? (
          <div className="flex-shrink-0 flex justify-end gap-4 p-4 bg-white dark:bg-gray-800 rounded-lg shadow">
            <button
              onClick={handleDiscard}
              className="text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white"
            >
              Discard
            </button>
            <button
              onClick={handleSubmit}
              className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 font-medium"
            >
              Save Changes
            </button>
          </div>
        ) : selectedLine && (
          <div className="flex-shrink-0">
            {(() => {
              // Check for blank/empty lines
              if (!selectedLine.line_text || selectedLine.line_text.trim() === '') {
                return null;
              }
              
              if (selectedLine.is_job === true && jobForLine) {
                return (
                  <JobCard 
                    job={jobForLine} 
                    mutate={jobsMutate} 
                    allJobs={jobs || []} 
                    showToast={showToast}
                    isMacOS={settings?.os === 'darwin'}
                    onJobChange={forceRevalidation}
                    crontabMutate={mutate}
                    selectedCrontab={selectedCrontab}
                    setSelectedCrontab={setSelectedCrontab}
                    readOnly={settings?.safe_mode}
                    settings={settings}
                  />
                );
              } else if (selectedLine.is_comment === true) {
                return <CommentCard line={selectedLine} />;
              } else if (selectedLine.is_env_var === true) {
                return <EnvVarCard line={selectedLine} />;
              } else {
                // For unknown line types, show as a comment
                return <CommentCard line={selectedLine} />;
              }
            })()}
          </div>
        )}
      </div>

      {showNewCrontab && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black bg-opacity-50">
          <NewCrontabOverlay
            formData={newCrontabForm}
            onFormChange={setNewCrontabForm}
            onClose={() => {
              setShowNewCrontab(false);
              setNewCrontabForm({
                filename: '',
                timezone: settings?.timezone || '',
                comments: ''
              });
            }}
            onCreateCrontab={handleCreateCrontab}
            timezones={settings?.timezones}
          />
        </div>
      )}

      {isToastVisible && (
        <Toast message={toastMessage} onClose={() => setIsToastVisible(false)} type={toastType} />
      )}
    </div>
  );
} 