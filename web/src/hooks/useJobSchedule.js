import { useState, useEffect } from 'react';
import guru, { getNextExecutionTimes } from '../lib/guru';

export function useJobSchedule(expression, timezone) {
  const [scheduleDescription, setScheduleDescription] = useState('');
  const [isValid, setIsValid] = useState(true);
  const [nextExecutionTimes, setNextExecutionTimes] = useState([]);

  useEffect(() => {
    if (!expression || typeof expression !== 'string' || !expression.trim()) {
      setScheduleDescription('Enter a valid cron schedule');
      setIsValid(false);
      return;
    }

    try {
      const description = guru(expression, timezone);
      setScheduleDescription(description);
      setIsValid(true);
    } catch (error) {
      console.error('Error parsing schedule:', error);
      setScheduleDescription('Invalid schedule format');
      setIsValid(false);
    }
  }, [expression, timezone]);

  useEffect(() => {
    const calculateTimes = () => {
      if (expression && isValid) {
        try {
          const nextTimes = getNextExecutionTimes(expression, timezone);
          setNextExecutionTimes(nextTimes);
        } catch (error) {
          console.error('Error calculating next execution times:', error);
          setNextExecutionTimes([]);
        }
      }
    };

    // Calculate immediately
    calculateTimes();

    // Calculate the time until the next minute
    const now = new Date();
    const msUntilNextMinute = (60 - now.getSeconds()) * 1000 - now.getMilliseconds();

    // Set initial timeout to align with the next minute
    const initialTimeout = setTimeout(() => {
      calculateTimes();
      // Then set up interval for every minute
      const interval = setInterval(calculateTimes, 60000);
      return () => clearInterval(interval);
    }, msUntilNextMinute);

    // Clean up timeout and interval on unmount or when dependencies change
    return () => {
      clearTimeout(initialTimeout);
    };
  }, [expression, isValid, timezone]);

  const validateSchedule = (schedule) => {
    // Basic cron expression validation
    // Format: * * * * * or @daily, @hourly, etc.
    const cronRegex = /^(@(annually|yearly|monthly|weekly|daily|hourly|reboot))|(@every (\d+(ns|us|µs|ms|s|m|h))+)|((((\d+,)+\d+|(\d+(\/|-)\d+)|\d+|\*) ?){5,7})$/;
    return cronRegex.test(schedule);
  };

  return {
    scheduleDescription,
    isValid,
    nextExecutionTimes,
    validateSchedule
  };
} 