import React from 'react';
import { SignupForm } from './SignupForm';
import cronitorScreenshot from '../../assets/cronitor-screenshot.png';

export function LearnMoreModal({ isOpen, onClose, onSignupSuccess, showToast, settings }) {
  if (!isOpen) return null;

  // Check if user already has an API key
  const hasApiKey = settings?.CRONITOR_API_KEY && settings.CRONITOR_API_KEY.trim() !== '';
  
  return (
    <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50" style={{ margin: '0px' }}>
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-6xl w-full mx-4 relative max-h-[90vh] overflow-y-auto">
        <button
          onClick={onClose}
          className="absolute top-0 right-8 bg-white dark:bg-gray-800 px-3 py-0 rounded-b-sm border border-t-0 border-gray-300 dark:border-gray-600 text-gray-400 hover:text-gray-500 dark:text-gray-400 dark:hover:text-gray-300 z-10 text-xl leading-none"
        >
          ×
        </button>
        <div className="p-8">
          <h2 className="text-3xl font-black text-gray-900 dark:text-white mb-8 text-left">Monitor your cron jobs with Cronitor</h2>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-12">
            <div>
              <div className="mb-8">
                <a href="https://cronitor.io/cron-job-monitoring?utm_source=cli&utm_campaign=modal&utm_content=1" target="_blank" rel="noopener noreferrer" className="block">
                  <img 
                    src={cronitorScreenshot} 
                    alt="Cronitor Dashboard" 
                    className="w-full rounded-lg shadow-lg"
                  />
                </a>
              </div>                
              <ul className="space-y-4">
                <li className="flex items-start">
                  <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true" data-slot="icon" className="h-6 w-6 text-green-500 mr-3 flex-shrink-0 mt-0.5">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                  </svg>
                  <span className="text-base text-gray-700 dark:text-gray-300 leading-relaxed">Instant alerts if a job fails or never starts.</span>
                </li>
                <li className="flex items-start">
                  <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true" data-slot="icon" className="h-6 w-6 text-green-500 mr-3 flex-shrink-0 mt-0.5">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                  </svg>
                  <span className="text-base text-gray-700 dark:text-gray-300 leading-relaxed">See the status, metrics and logs from every job.</span>
                </li>
                <li className="flex items-start">
                  <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" aria-hidden="true" data-slot="icon" className="h-6 w-6 text-green-500 mr-3 flex-shrink-0 mt-0.5">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                  </svg>
                  <span className="text-base text-gray-700 dark:text-gray-300 leading-relaxed">Track performance with a year of data retention.</span>
                </li>
              </ul>
            </div>
            {hasApiKey ? (
              <div className="bg-gray-50 dark:bg-gray-900 p-6 rounded-lg flex flex-col justify-center">
                <div className="flex items-center mb-4">
                  <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" className="h-8 w-8 text-green-500 mr-3 flex-shrink-0">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z"></path>
                  </svg>
                  <h3 className="text-2xl font-bold text-gray-900 dark:text-white">You're all set</h3>
                </div>
                <p className="text-base text-gray-700 dark:text-gray-300 mb-6">
                  You have already connected your Cronitor account. To monitor this job, just flip the toggle switch.
                </p>
                <div>
                  <a 
                    href="https://cronitor.io/app" 
                    target="_blank" 
                    rel="noopener noreferrer" 
                    className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
                  >
                    Show Cronitor Dashboard
                    <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor" className="ml-2 -mr-1 w-4 h-4">
                      <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 4.5 21 12m0 0-7.5 7.5M21 12H3" />
                    </svg>
                  </a>
                </div>
              </div>
            ) : (
              <div className="bg-gray-50 dark:bg-gray-900 p-6 rounded-lg">
                <SignupForm 
                  onSuccess={(apiKeys) => {
                    // Show immediate success toast
                    if (showToast) {
                      showToast('Account created successfully! Welcome to Cronitor.', 'success');
                    }
                    // Call the parent's success handler
                    if (onSignupSuccess) {
                      onSignupSuccess(apiKeys);
                    }
                  }}
                  onError={(error) => {
                    if (showToast) {
                      showToast(error, 'error');
                    } else {
                      console.error('Signup error:', error);
                    }
                  }}
                />
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
} 