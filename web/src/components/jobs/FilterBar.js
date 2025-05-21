import React from 'react';

// Filter options for the job filters
export const FILTER_OPTIONS = [
  { id: 'active', label: 'Active', roundedStyle: 'rounded-l-md' },
  { id: 'suspended', label: 'Suspended', roundedStyle: 'rounded-r-md' },
  { id: 'running', label: 'Running' },
  { id: 'monitored', label: 'Monitored', roundedStyle: 'rounded-l-md' },
  { id: 'unmonitored', label: 'Unmonitored', roundedStyle: 'rounded-r-md' }
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

    let buttonClasses = "px-3 py-3 text-base rounded-md bg-gray-100 dark:bg-gray-800 flex-shrink-0 flex items-center ";
    if (activeFilters[filter.id]) {
      buttonClasses += 'text-purple-600 dark:text-purple-400';
    } else {
      buttonClasses += 'text-gray-500 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-400';
    }
    
    // Apply specific rounding if defined in FILTER_OPTIONS
    if (filter.roundedStyle) {
      buttonClasses = buttonClasses.replace('rounded-md', filter.roundedStyle);
    }

    return (
      <button
        key={filter.id}
        onClick={() => handleFilterToggle(filter.id)}
        className={buttonClasses}
        style={ (filterId === 'suspended' || filterId === 'unmonitored') ? { marginLeft: '-1px', borderLeft: '1px solid rgba(128,128,128,0.2)' } : {} }
      >
        {filter.label}
      </button>
    );
  };

  return (
    <div className="flex items-center gap-3 flex-nowrap min-w-0 justify-end"> {/* Main gap between groups/elements */}
      {/* Group 1: Active/Suspended */}
      <div className="flex items-center">
        {renderFilterButton('active')}
        {renderFilterButton('suspended')}
      </div>

      {/* Group 2: Running (Standalone) */}
      {renderFilterButton('running')}

      {/* Group 3: Monitored/Unmonitored */}
      <div className="flex items-center">
        {renderFilterButton('monitored')}
        {renderFilterButton('unmonitored')}
      </div>
      
      <input
        type="text"
        value={inputValue}
        onChange={onInputChange}
        placeholder="Search..."
        className="px-3 py-3 w-64 ml-2 flex-shrink-0 bg-gray-100 dark:bg-gray-800 dark:text-gray-100 rounded-md text-base placeholder-gray-500 dark:placeholder-gray-500 focus:outline-none"
      />
    </div>
  );
} 