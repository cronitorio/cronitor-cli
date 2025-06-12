function padded2 (number) {
  return ('0' + number).slice(-2)
}

function getTimezoneOffset (date, browserTimezone, jobTimezone) {
  const browserDate = new Date(date.toLocaleString('en-US', { timeZone: browserTimezone }));
  const jobDate = new Date(date.toLocaleString('en-US', { timeZone: jobTimezone }));
  const diff = jobDate.getTime() - browserDate.getTime();
  const hours = Math.abs(Math.floor(diff / (1000 * 60 * 60)));
  const minutes = Math.abs(Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60)));
  const sign = diff > 0 ? '-' : '+';
  return {
    offset: `${sign}${hours}:${minutes.toString().padStart(2, '0')}`,
    diff: diff
  };
}

function applyOffset (date, offset) {
  return new Date(date.getTime() - offset);
}

function getTimezoneAbbr (timezone) {
  try {
    return new Date().toLocaleTimeString('en-US', {
      timeZone: timezone,
      timeZoneName: 'short'
    }).split(' ')[2];
  } catch (error) {
    console.error('Error getting timezone abbreviation:', error);
    return timezone; // Fallback to full timezone name
  }
};


export default function dateFormatter(date, jobTimezone) {
  let localZone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const offset = getTimezoneOffset(new Date(), localZone, jobTimezone)
  const localDate = applyOffset(
    new Date(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate(), date.getUTCHours(), date.getUTCMinutes(), date.getUTCSeconds()), 
    offset.diff
  )

  return {
    job: {
      year: date.getUTCFullYear(),
      month: padded2(date.getUTCMonth() + 1),
      date: padded2(date.getUTCDate()),
      hour: padded2(date.getUTCHours()),
      minute: padded2(date.getUTCMinutes()),
      second: padded2(date.getUTCSeconds()),
      zone: getTimezoneAbbr(jobTimezone)
    },

    local: {
      year: localDate.getFullYear(),
      month: padded2(localDate.getMonth() + 1),
      date: padded2(localDate.getDate()),
      hour: padded2(localDate.getHours()),
      minute: padded2(localDate.getMinutes()),
      second: padded2(localDate.getSeconds()),
      zone: getTimezoneAbbr(localZone)
    }
  }
}
