import React, { useState, useEffect, useRef, useCallback } from 'react';
import useSWR from 'swr';
import { NewCrontabOverlay } from './jobs/NewCrontabOverlay';
import { Toast } from './Toast';
import { JobCard } from './jobs/JobCard';
import { CommentCard } from './crontabs/CommentCard';
import { EnvVarCard } from './crontabs/EnvVarCard';
import { useLocation, useSearchParams } from 'react-router-dom';
import { csrfFetcher, csrfFetch } from '../utils/api';

export default function Crontabs() {
  const location = useLocation();
  const isCrontabsView = location.pathname === '/crontabs';
  const [searchParams, setSearchParams] = useSearchParams();
  
  const [revalidationKey, setRevalidationKey] = useState(0);
  const [isEditing, setIsEditing] = useState(false);
  const [editedContent, setEditedContent] = useState('');
  const textareaRef = useRef(null);
  const scrollContainerRef = useRef(null);
  
  // Settings data - minimal refresh
  const { data: settings } = useSWR('/api/settings', csrfFetcher, {
    revalidateOnFocus: false,
    refreshInterval: 0
  });
  
  // Primary data for Crontabs view - fast refresh
  const { data: crontabs, error, mutate } = useSWR(
    `/api/crontabs?key=${revalidationKey}`,
    csrfFetcher,
    {
      refreshInterval: 5000, // Keep fast refresh for primary data
      revalidateOnFocus: true,
      dedupingInterval: 5000 // 5 seconds deduplication to match refresh
    }
  );
  
  // Jobs data - slower refresh since it's secondary on Crontabs view
  const { data: jobs, mutate: jobsMutate } = useSWR(
    crontabs ? `/api/jobs?key=${revalidationKey}` : null, 
    csrfFetcher, 
    {
      refreshInterval: isCrontabsView ? 30000 : 5000, // 30s on Crontabs view, 5s elsewhere
      revalidateOnFocus: true,
      dedupingInterval: isCrontabsView ? 30000 : 5000 // Match refresh interval
    }
  );
  
  // User data - minimal refresh since it changes infrequently
  const { data: users } = useSWR('/api/users', csrfFetcher, {
    refreshInterval: 0, // No auto-refresh - users rarely change
    revalidateOnFocus: true, // Revalidate when window gets focus
    revalidateOnReconnect: false,
    dedupingInterval: 300000 // 5 minutes deduplication
  });
  
  const [selectedCrontab, setSelectedCrontab] = useState(null);
  const [selectedLine, setSelectedLine] = useState(null);
  const [showNewCrontab, setShowNewCrontab] = useState(false);
  const [isToastVisible, setIsToastVisible] = useState(false);
  const [toastMessage, setToastMessage] = useState('');
  const [toastType, setToastType] = useState('error');
  
  const [newCrontabForm, setNewCrontabForm] = useState({
    filename: '',
    timezone: '',
    comments: ''
  });
  const [isReloading, setIsReloading] = useState(false);

  // Update timezone when settings are loaded
  useEffect(() => {
    if (settings?.timezone) {
      setNewCrontabForm(prev => ({
        ...prev,
        timezone: settings.timezone
      }));
    }
  }, [settings]);

  // Select crontab from URL or first crontab when data loads
  useEffect(() => {
    if (crontabs && crontabs.length > 0) {
      const crontabFromURL = searchParams.get('crontab');
      const lineFromURL = searchParams.get('line');
      
      if (crontabFromURL && !selectedCrontab) {
        // Try to find the crontab from URL
        const foundCrontab = crontabs.find(c => c.filename === crontabFromURL);
        if (foundCrontab) {
          setSelectedCrontab(foundCrontab);
          
          // If we also have a line number, select it after crontab is set
          if (lineFromURL) {
            const lineNumber = parseInt(lineFromURL, 10);
            if (!isNaN(lineNumber) && foundCrontab.lines) {
              // Find the line with matching line_number (1-indexed)
              const lineToSelect = foundCrontab.lines.find(line => line.line_number === lineNumber);
              if (lineToSelect) {
                setSelectedLine(lineToSelect);
              }
            }
          }
          // Mark initial load as complete after processing URL params
          setIsInitialLoad(false);
        } else if (!selectedCrontab) {
          // Crontab from URL not found, select first one
          setSelectedCrontab(crontabs[0]);
          setIsInitialLoad(false);
        }
      } else if (!selectedCrontab) {
        // No URL params, select first crontab
        setSelectedCrontab(crontabs[0]);
        setIsInitialLoad(false);
      } else {
        // We already have a selected crontab, initial load is done
        setIsInitialLoad(false);
      }
    }
  }, [crontabs, searchParams]); // eslint-disable-line react-hooks/exhaustive-deps
  // Note: selectedCrontab is intentionally omitted to avoid infinite loop when setting from URL params

  // Clear selectedLine when selectedCrontab changes
  useEffect(() => {
    if (selectedCrontab) {
      // Clear the selected line when switching crontabs
      // (unless we're setting both from URL params, which is handled in the previous effect)
      const lineFromURL = searchParams.get('line');
      if (!lineFromURL) {
        setSelectedLine(null);
      }
    }
  }, [searchParams]); // eslint-disable-line react-hooks/exhaustive-deps
  // Note: selectedCrontab is intentionally omitted to avoid clearing the line when it's set from URL params

  // Track if we're still loading initial data from URL
  const [isInitialLoad, setIsInitialLoad] = useState(true);
  
  // Update URL when selectedCrontab or selectedLine changes
  useEffect(() => {
    // Skip URL updates during initial load to prevent clearing URL params
    if (isInitialLoad) return;
    
    const newParams = new URLSearchParams();
    
    if (selectedCrontab) {
      newParams.set('crontab', selectedCrontab.filename);
    }
    
    if (selectedLine && selectedCrontab) {
      // Use the line_number (1-indexed) instead of array index
      if (selectedLine.line_number !== undefined && selectedLine.line_number !== null) {
        newParams.set('line', selectedLine.line_number.toString());
      }
    }
    
    // Only update if the string form of params actually changes
    if (newParams.toString() !== searchParams.toString()) {
      setSearchParams(newParams, { replace: true });
    }
  }, [selectedCrontab, selectedLine, setSearchParams, searchParams, isInitialLoad]);

  const showToast = (message, type = 'error') => {
    setToastMessage(message);
    setToastType(type);
    setIsToastVisible(true);
    setTimeout(() => setIsToastVisible(false), type === 'error' ? 6000 : 3000);
  };

  const handleReload = async () => {
    setIsReloading(true);
    await mutate();
    // Ensure loading state shows for at least 1 second
    setTimeout(() => setIsReloading(false), 1000);
  };

  const handleCreateCrontab = async () => {
    try {
      if (!newCrontabForm.timezone && settings?.timezone) {
        setNewCrontabForm(prev => ({
          ...prev,
          timezone: settings.timezone
        }));
      }
      
      const response = await csrfFetch('/api/crontabs', {
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

  const formatCrontabContent = useCallback(() => {
    if (!selectedCrontab || !selectedCrontab.lines) return [];
    
    const formattedLines = [];
    let lineNumber = 1;
    
    selectedCrontab.lines.forEach((line) => {
      // If this is a job line that's ignored, add the cronitor: ignore comment first
      if (line.is_job && line.ignored) {
        formattedLines.push({
          lineNumber: lineNumber++,
          text: `# cronitor: ignore`,
          originalLine: line,
          isCronitorIgnoreComment: true
        });
      }
      
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
        isNameComment: false,
        isCronitorIgnoreComment: false
      });
    });
    
    return formattedLines;
  }, [selectedCrontab]);

  // Scroll to selected line when it changes
  useEffect(() => {
    if (selectedLine && scrollContainerRef.current && !isEditing) {
      // Find the index of the selected line in the formatted content
      const formattedLines = formatCrontabContent();
      const selectedIndex = formattedLines.findIndex(fl => fl.originalLine === selectedLine);
      
      if (selectedIndex !== -1) {
        // Calculate the scroll position based on line height (1.5 * font-size)
        // Assuming font-size is 14px (text-sm), line height is 21px
        const lineHeight = 21; // 1.5 * 14px
        const paddingTop = 16; // py-4 = 1rem = 16px
        const scrollTop = selectedIndex * lineHeight + paddingTop;
        
        // Scroll to position with some offset to center the line
        const containerHeight = scrollContainerRef.current.clientHeight;
        const targetScroll = scrollTop - (containerHeight / 2) + (lineHeight / 2);
        
        scrollContainerRef.current.scrollTo({
          top: Math.max(0, targetScroll),
          behavior: 'smooth'
        });
      }
    }
  }, [selectedLine, isEditing, selectedCrontab, formatCrontabContent]);

  // Get or create a job object for the selected line
  const getJobForLine = () => {
    if (!selectedLine || !selectedLine.is_job) return null;
    
    // If the line has a job object from the server, use it
    if (selectedLine.job) {
      // Add any missing fields that might be needed
      const job = { ...selectedLine.job };
      
      // Ensure line_number is set from the selectedLine if not in job
      if (!job.line_number && selectedLine.line_number !== undefined) {
        job.line_number = selectedLine.line_number;
      }
      
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
    
    // Get all lines including Name comments and cronitor: ignore comments (same as what's displayed)
    const lines = selectedCrontab.lines.map(line => {
      const result = [];
      
      // Add cronitor: ignore comment if the job is ignored
      if (line.is_job && line.ignored) {
        result.push('# cronitor: ignore');
      }
      
      // Add Name comment if the job has a name
      if (line.is_job && line.name) {
        result.push(`# Name: ${line.name}`);
      }
      
      // Add the actual line
      let lineText = line.line_text;
      if (line.is_comment && lineText && !lineText.startsWith('#')) {
        lineText = `# ${lineText}`;
      }
      result.push(lineText);
      
      return result;
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
      let currentIgnored = false;

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        
        // Check if this is a cronitor: ignore comment
        const ignoreMatch = line.match(/^#\s*cronitor:\s*ignore\s*$/i);
        if (ignoreMatch) {
          currentIgnored = true;
          // Don't add the cronitor: ignore comment to processed lines
          continue;
        }
        
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
          (line.startsWith('#') && line.trim().length > 1 && !line.match(/^#\s*Name:/i) && !line.match(/^#\s*cronitor:\s*ignore/i)) // Suspended job line (but not a Name or cronitor comment)
        );

        if (isJobLine) {
          processedLines.push({
            line_text: line,
            name: currentName || undefined,
            ignored: currentIgnored || undefined
          });
          currentName = null; // Reset the name after associating it
          currentIgnored = false; // Reset the ignored flag after associating it
        } else {
          // Regular comment, empty line, or environment variable
          processedLines.push({ line_text: line });
        }
      }
      
      const response = await csrfFetch(`/api/crontabs/${selectedCrontab.filename}`, {
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
            {(error.message.includes('Unable to connect') || error.message.includes('Load failed')) && (
              <div className="mt-2 text-sm text-red-700 dark:text-red-300">
                <p>Possible causes:</p>
                <ul className="list-disc list-inside mt-1">
                <li>The dash service is not running</li>
                <li>Your IP is not whitelisted</li>
                <li>An SSH tunnel or VPN connection is required</li>
                <li>Server is restarting</li>
              </ul>
              </div>
            )}
            <div className="mt-4">
              <button
                onClick={handleReload}
                disabled={isReloading}
                className="inline-flex items-center px-4 py-2 border border-gray-300 dark:border-gray-600 text-sm font-medium rounded-md shadow-sm text-gray-700 dark:text-gray-200 bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isReloading ? (
                  <>
                    <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-gray-500 dark:text-gray-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                    </svg>
                    Reloading...
                  </>
                ) : (
                  <>
                    <svg className="mr-2 -ml-1 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                    Reload
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  if (!crontabs) {
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
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow" style={{ height: '60vh', overflow: 'hidden' }}>
          <div className="h-full rounded-lg" style={{ backgroundColor: 'rgba(0, 0, 0, 0.9)', overflow: 'hidden' }}>
            <div ref={scrollContainerRef} className="h-full overflow-y-auto flex">
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
                <div className="flex-1">
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
                          // Skip name comment and cronitor: ignore lines
                          while (newIndex >= 0 && (formattedLines[newIndex].isNameComment || formattedLines[newIndex].isCronitorIgnoreComment)) {
                            newIndex--;
                          }
                          e.preventDefault();
                        } else if (e.key === 'ArrowDown' && currentIndex < formattedLines.length - 1) {
                          newIndex = currentIndex + 1;
                          // Skip name comment and cronitor: ignore lines
                          while (newIndex < formattedLines.length && (formattedLines[newIndex].isNameComment || formattedLines[newIndex].isCronitorIgnoreComment)) {
                            newIndex++;
                          }
                          e.preventDefault();
                        }
                        
                        if (newIndex !== currentIndex && newIndex >= 0 && newIndex < formattedLines.length && !formattedLines[newIndex].isNameComment && !formattedLines[newIndex].isCronitorIgnoreComment) {
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
                              !line.isNameComment && !line.isCronitorIgnoreComment ? 'hover:bg-gray-900 cursor-pointer' : 'hover:bg-gray-900 cursor-pointer'
                            } ${
                              line.originalLine === selectedLine ? 'bg-blue-900/30' : ''
                            } ${
                              line.isNameComment || line.isCronitorIgnoreComment || (line.originalLine && line.originalLine.is_comment) ? 'text-gray-400' : ''
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
                    users={users || []}
                    crontabs={crontabs || []}
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