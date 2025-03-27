import React, { useEffect, useState } from 'react';
import { BrowserRouter as Router, Routes, Route, Link } from 'react-router-dom';
import { DocumentIcon, ClockIcon, Cog6ToothIcon, SunIcon, MoonIcon } from '@heroicons/react/24/outline';
import Cookies from 'js-cookie';
import cronitorLogo from './assets/cronitor.png';

const navigation = [
  { name: 'Jobs', href: '/', icon: ClockIcon },
  { name: 'Crontabs', href: '/crontabs', icon: DocumentIcon },
  { name: 'Settings', href: '/settings', icon: Cog6ToothIcon },
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
          <div className="hidden md:flex md:w-64 md:flex-col">
            <div className="flex min-h-0 flex-1 flex-col border-r border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
              <div className="flex flex-1 flex-col overflow-y-auto pt-5 pb-4">
                <div className="flex flex-shrink-0 items-center px-4">
                  <img 
                    src={cronitorLogo} 
                    alt="LocalDash Logo" 
                    className="h-8 w-8 object-contain"
                  />
                  <span className="ml-3 text-xl font-bold text-gray-900 dark:text-white">LocalDash</span>
                </div>
                <nav className="mt-5 flex-1 space-y-1 bg-white dark:bg-gray-800 px-2">
                  {navigation.map((item) => (
                    <Link
                      key={item.name}
                      to={item.href}
                      className="group flex items-center px-2 py-2 text-sm font-medium rounded-md text-gray-600 dark:text-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700 hover:text-gray-900 dark:hover:text-white"
                    >
                      <item.icon
                        className="mr-3 h-6 w-6 flex-shrink-0 text-gray-400 dark:text-gray-500 group-hover:text-gray-500 dark:group-hover:text-gray-400"
                        aria-hidden="true"
                      />
                      {item.name}
                    </Link>
                  ))}
                </nav>
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
          </div>

          {/* Main content */}
          <div className="flex flex-1 flex-col overflow-hidden">
            <main className="flex-1 overflow-y-auto">
              <div className="py-6">
                <div className="mx-auto max-w-7xl px-4 sm:px-6 md:px-8">
                  <Routes>
                    <Route path="/" element={<Jobs />} />
                    <Route path="/crontabs" element={<Crontabs />} />
                    <Route path="/settings" element={<Settings />} />
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

function Jobs() {
  return (
    <div>
      <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Jobs</h1>
    </div>
  );
}

function Crontabs() {
  return (
    <div>
      <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Crontabs</h1>
    </div>
  );
}

function Settings() {
  return (
    <div>
      <h1 className="text-2xl font-semibold text-gray-900 dark:text-white">Settings</h1>
    </div>
  );
}

export default App; 