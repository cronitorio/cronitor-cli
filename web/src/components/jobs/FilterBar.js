import React from 'react';

// Filter options for the job filters
export const FILTER_OPTIONS = [
  { id: 'active', label: 'Active' },
  { id: 'suspended', label: 'Suspended' },
  { id: 'running', label: 'Running' },
  { id: 'monitored', label: 'Monitored' },
  { id: 'unmonitored', label: 'Unmonitored' }
];

export function FilterBar({ activeFilters, setActiveFilters, inputValue, onInputChange }) {
  const handleFilterToggle = (filterId) => {
    setActiveFilters(prev => {
      const newActiveFilters = { ...prev };
      const currentlySelected = prev[filterId];
      newActiveFilters[filterId] = !currentlySelected;
      if (newActiveFilters[filterId]) {
        if (filterId === 'active') newActiveFilters.suspended = false;
        else if (filterId === 'suspended') newActiveFilters.active = false;
        else if (filterId === 'monitored') newActiveFilters.unmonitored = false;
        else if (filterId === 'unmonitored') newActiveFilters.monitored = false;
      }
      return newActiveFilters;
    });
  };

  const renderFilterButton = (filterId) => {
    const filter = FILTER_OPTIONS.find(f => f.id === filterId);
    if (!filter) return null;

    let buttonClasses = "px-3 py-3 text-base rounded-md bg-gray-200 dark:bg-gray-800 flex-shrink-0 flex items-center ";
    if (activeFilters[filter.id]) {
      buttonClasses += 'text-purple-600 dark:text-purple-400 border border-purple-200 dark:border-purple-800';
    } else {
      buttonClasses += 'text-gray-500 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-400 border border-transparent';
    }

    return (
      <button
        key={filter.id}
        onClick={() => handleFilterToggle(filter.id)}
        className={buttonClasses}
      >
        {filter.label}
      </button>
    );
  };

  return (
    <div className="flex items-center gap-3 flex-nowrap min-w-0 justify-end">
      {renderFilterButton('active')}
      {renderFilterButton('suspended')}
      {renderFilterButton('running')}
      {renderFilterButton('monitored')}
      {renderFilterButton('unmonitored')}
      
      <div className="relative flex-shrink-0">
        <input
          type="text"
          value={inputValue}
          onChange={onInputChange}
          placeholder="Search..."
          className="px-3 py-3 pr-8 w-full bg-gray-200 dark:bg-gray-800 text-gray-900 dark:text-gray-100 rounded-md text-base placeholder-gray-500 dark:placeholder-gray-500 focus:outline-none"
        />
        {inputValue && (
          <button
            onClick={() => onInputChange({ target: { value: '' } })}
            className="absolute right-2 top-1/2 transform -translate-y-1/2 text-gray-500 hover:text-gray-700 dark:text-gray-500 dark:hover:text-gray-300"
            aria-label="Clear search"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        )}
      </div>
    </div>
  );
} 
