import React, { useEffect, useState } from 'react';
import { BrowserRouter as Router, Routes, Route, Link, useLocation } from 'react-router-dom';
import { ClockIcon, Cog6ToothIcon, SunIcon, MoonIcon, FolderOpenIcon, DocumentTextIcon } from '@heroicons/react/24/outline';
import Cookies from 'js-cookie';
import cronitorLogo from './assets/cronitor.png';
import Settings from './components/Settings';
import useSWR from 'swr';
import Jobs from './components/Jobs';
import Crontabs from './components/Crontabs';
import Docs from './components/Docs';
import { usePrefetch } from './hooks/usePrefetch';
import { csrfFetcher, csrfFetch } from './utils/api';

const navigation = [
  { name: 'Jobs', href: '/', icon: ClockIcon },
  { name: 'Crontabs', href: '/crontabs', icon: FolderOpenIcon },
  { name: 'Settings', href: '/settings', icon: Cog6ToothIcon },
  { name: 'Docs', href: '/docs', icon: DocumentTextIcon },
];

function ToggleSwitch({ isOn, onChange }) {
  return (
    <button
      role="switch"
      aria-checked={isOn}
      onClick={onChange}
      className={`
        relative inline-flex h-6 w-11 items-center rounded-full transition-colors duration-200 ease-in-out
        ${isOn ? 'bg-blue-500' : 'bg-gray-200 dark:bg-gray-700'}
      `}
    >
      <span
        className={`
          inline-block h-4 w-4 transform rounded-full bg-white transition-transform duration-200 ease-in-out
          ${isOn ? 'translate-x-6' : 'translate-x-1'}
        `}
      />
    </button>
  );
}

function Sidebar({ isDark, toggleTheme }) {
  const location = useLocation();
  const { data } = useSWR('/api/settings', csrfFetcher, {
    refreshInterval: 30000, // Reduced from 5s to 30s - settings don't change often
    revalidateOnFocus: false
  });
  const { prefetchCrontabs, prefetchJobs } = usePrefetch();
  const [isUpdating, setIsUpdating] = useState(false);
  const [updateProgress, setUpdateProgress] = useState('');

  const handleNavHover = (href) => {
    // Prefetch data when hovering over navigation links
    if (href === '/crontabs') {
      prefetchCrontabs();
    } else if (href === '/') {
      prefetchJobs();
    }
  };

  const handleUpdate = async () => {
    if (isUpdating) return;
    
    setIsUpdating(true);
    setUpdateProgress('Starting update...');

    try {
      const response = await csrfFetch('/api/update/perform', {
        method: 'POST',
      });

      if (!response.ok) {
        throw new Error(`Update failed: ${response.status} ${response.statusText}`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value);
        const lines = chunk.split('\n');
        
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6));
              if (data.progress) {
                setUpdateProgress(data.progress);
              } else if (data.error) {
                throw new Error(data.error);
              }
            } catch (e) {
              // Ignore JSON parse errors for invalid lines
            }
          }
        }
      }

      // The server should restart automatically, so we can show a message and reload the page
      setUpdateProgress('Update complete! Page will reload...');
      setTimeout(() => {
        window.location.reload();
      }, 3000);
      
    } catch (error) {
      console.error('Update failed:', error);
      setUpdateProgress(`Update failed: ${error.message}`);
      setIsUpdating(false);
    }
  };

  return (
    <div className="hidden md:flex md:w-64 md:flex-col">
      <div className="flex min-h-0 flex-1 flex-col border-r border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <div className="flex flex-1 flex-col overflow-y-auto pt-5 pb-4">
          <div className="flex flex-shrink-0 items-center px-4">
            <img 
              src={cronitorLogo} 
              alt="LocalDash Logo" 
              className="h-8 w-8 object-contain"
            />
            <span className="ml-3 text-xl font-bold text-gray-900 dark:text-white">Crontab Guru</span>
          </div>
          <nav className="mt-5 flex-1 space-y-1 bg-white dark:bg-gray-800 px-2">
            {navigation.map((item) => {
              const isActive = location.pathname === item.href;
              return (
                <Link
                  key={item.name}
                  to={item.href}
                  onMouseEnter={() => handleNavHover(item.href)}
                  className={`group flex items-center px-2 py-2 text-sm font-medium rounded-md ${
                    isActive
                      ? 'bg-gray-100 dark:bg-gray-700 text-gray-900 dark:text-white'
                      : 'text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 hover:text-gray-900 dark:hover:text-white'
                  }`}
                >
                  <item.icon
                    className={`mr-3 h-6 w-6 flex-shrink-0 ${
                      isActive
                        ? 'text-gray-500 dark:text-gray-400'
                        : 'text-gray-400 dark:text-gray-500 group-hover:text-gray-500 dark:group-hover:text-gray-400'
                    }`}
                    aria-hidden="true"
                  />
                  {item.name}
                </Link>
              );
            })}
          </nav>
        </div>
        {/* Version Info */}
        <div className="flex flex-col">
          <div className="border-t border-gray-200 dark:border-gray-700">
            <div className="px-4 py-2">
              <div className="text-sm text-gray-500 dark:text-gray-400">
                {data?.hostname || '...'}
              </div>
            </div>
          </div>
          <div className="border-t border-gray-200 dark:border-gray-700">
            <div className="px-4 py-2">
              <div className="flex items-center justify-between">
                <a
                  href="https://cronitor.io/docs/using-cronitor-cli"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-sm text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300"
                >
                  CronitorCLI v{data?.version || '...'}
                </a>
                {/* Update Button */}
                {data?.update_status && data.update_status.has_update && !data?.safe_mode && (
                  <button
                    onClick={handleUpdate}
                    disabled={isUpdating}
                    className={`px-2 py-1 text-xs font-medium rounded transition-colors ${
                      isUpdating
                        ? 'bg-gray-100 dark:bg-gray-700 text-gray-500 dark:text-gray-400 cursor-not-allowed'
                        : 'bg-blue-500 text-white hover:bg-blue-600 focus:outline-none focus:ring-1 focus:ring-blue-500'
                    }`}
                  >
                    {isUpdating ? 'UPDATING...' : 'UPDATE'}
                  </button>
                )}
              </div>
              {/* Update Progress */}
              {updateProgress && (
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {updateProgress}
                </div>
              )}
              {/* Safe Mode Message */}
              {data?.safe_mode && data?.update_status?.has_update && (
                <div className="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  Update available (v{data.update_status.latest_version}) - Updates disabled in safe mode
                </div>
              )}
            </div>
          </div>
        </div>
        {/* Theme Toggle */}
        <div className="px-4 py-4 border-t border-gray-200 dark:border-gray-700">
          <div className="flex items-center justify-between">
            <div className="flex items-center">
              {isDark ? (
                <MoonIcon className="h-5 w-5 text-gray-400 dark:text-gray-500" />
              ) : (
                <SunIcon className="h-5 w-5 text-gray-400 dark:text-gray-500" />
              )}
              <span className="ml-3 text-sm font-medium text-gray-600 dark:text-gray-300">
                {isDark ? 'Dark Mode' : 'Light Mode'}
              </span>
            </div>
            <ToggleSwitch isOn={isDark} onChange={toggleTheme} />
          </div>
        </div>
      </div>
    </div>
  );
}

function App() {
  const [isDark, setIsDark] = useState(true);

  useEffect(() => {
    // Check for saved theme preference
    const savedTheme = Cookies.get('theme');
    if (savedTheme) {
      setIsDark(savedTheme === 'dark');
    }
    // Apply theme on mount
    document.documentElement.classList.toggle('dark', isDark);
  }, [isDark]);

  const toggleTheme = () => {
    const newTheme = !isDark;
    setIsDark(newTheme);
    Cookies.set('theme', newTheme ? 'dark' : 'light', { expires: 365 });
    document.documentElement.classList.toggle('dark', newTheme);
  };

  return (
    <Router>
      <div className="min-h-screen bg-gray-100 dark:bg-gray-900 font-mono">
        <div className="flex h-screen">
          {/* Sidebar */}
          <Sidebar isDark={isDark} toggleTheme={toggleTheme} />

          {/* Main content */}
          <div className="flex flex-1 flex-col overflow-hidden">
            <main className="flex-1 overflow-y-auto">
              <div className="py-6">
                <div className="mx-auto max-w-[84rem] px-4 sm:px-6 md:px-8">
                  <Routes>
                    <Route path="/" element={<Jobs />} />
                    <Route path="/crontabs" element={<Crontabs />} />
                    <Route path="/settings" element={<Settings />} />
                    <Route path="/docs" element={<Docs />} />
                  </Routes>
                </div>
              </div>
            </main>
          </div>
        </div>
      </div>
    </Router>
  );
}

export default App; 