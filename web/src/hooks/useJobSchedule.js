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

    // If the expression is being edited, update more frequently
    if (expression && isValid) {
      const interval = setInterval(calculateTimes, 1000);
      return () => clearInterval(interval);
    }

    // For non-editing state, update every minute
    const now = new Date();
    const msUntilNextMinute = (60 - now.getSeconds()) * 1000 - now.getMilliseconds();

    const initialTimeout = setTimeout(() => {
      calculateTimes();
      const interval = setInterval(calculateTimes, 60000);
      return () => clearInterval(interval);
    }, msUntilNextMinute);

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