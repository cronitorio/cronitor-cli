function roundUpMillis(date) {
  const millis = date.getUTCMilliseconds();
  return millis !== 0 ? new Date(date.getTime() + (1000 - millis)) : date;
}

function roundUpSeconds(date) {
  const nextDate = roundUpMillis(date);
  const seconds = nextDate.getUTCSeconds();
  return seconds !== 0 ? new Date(nextDate.getTime() + ((60 - seconds) * 1000)) : nextDate;
}

function utcDate(year, month, date, hours, minutes) {
  return new Date(Date.UTC(year, month, date, hours, minutes));
}

function nextDate(normalizedSchedule, startDate, invocation) {
  if (invocation > (12 + 31 + 24 + 60)) {  // to protect against endless recursion in case of a bug
    return null;
  }

  const month = startDate.getUTCMonth() + 1;
  const year = startDate.getUTCFullYear();
  if (!normalizedSchedule.months.includes(month)) {
    return nextDate(normalizedSchedule, utcDate(year, month + 1 - 1, 1, 0, 0), ++invocation);
  }

  const date = startDate.getUTCDate();
  const weekday = startDate.getUTCDay();
  const dateMatches = normalizedSchedule.dates.includes(date);
  const weekdayMatches = normalizedSchedule.weekdays.includes(weekday);
  if ((normalizedSchedule.daysAnded && (!dateMatches || !weekdayMatches))
    || (!normalizedSchedule.daysAnded && (!dateMatches && !weekdayMatches))) {
    return nextDate(normalizedSchedule, utcDate(year, month - 1, date + 1, 0, 0), ++invocation);
  }

  const hours = startDate.getUTCHours();
  if (!normalizedSchedule.hours.includes(hours)) {
    return nextDate(normalizedSchedule, utcDate(year, month - 1, date, hours + 1, 0), ++invocation);
  }

  const minutes = startDate.getUTCMinutes();
  if (!normalizedSchedule.minutes.includes(minutes)) {
    return nextDate(normalizedSchedule, utcDate(year, month - 1, date, hours, minutes + 1), ++invocation);
  }

  return startDate;
}

module.exports = function(normalizedSchedule, startDate) {
  if (!normalizedSchedule || typeof normalizedSchedule !== 'object') {
    return null;
  }

  // Check if all required arrays exist and have length
  const requiredArrays = ['months', 'dates', 'weekdays', 'hours', 'minutes'];
  for (const arrayName of requiredArrays) {
    if (!Array.isArray(normalizedSchedule[arrayName]) || normalizedSchedule[arrayName].length === 0) {
      return null;
    }
  }

  // Check if daysAnded property exists
  if (typeof normalizedSchedule.daysAnded !== 'boolean') {
    return null;
  }

  return nextDate(normalizedSchedule, roundUpSeconds(startDate), 1);
};
