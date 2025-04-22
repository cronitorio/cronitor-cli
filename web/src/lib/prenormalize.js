'use strict';

function replaceFromHash(string, hash) {
  const saveReplace = (text, searchText, replacement) => {
    const regex = new RegExp(`(^|[ ,-/])${searchText}($|[ ,-/])`, 'gi');
    const fullReplacement = `$1${replacement}$2`;
    return text
    .replace(regex, fullReplacement)
    .replace(regex, fullReplacement); // twice due to potentially overlapping separators such as in '*,*'
  }
  return Object.keys(hash).reduce((text, key) => saveReplace(text, key, hash[key]), string);
}

const WEEKDAYS = {
  sun: '0',
  mon: '1',
  tue: '2',
  wed: '3',
  thu: '4',
  fri: '5',
  sat: '6'
};
function substituteWeekdays(part) {
  return replaceFromHash(part, WEEKDAYS);
}

const MONTHS = {
  jan: '1',
  feb: '2',
  mar: '3',
  apr: '4',
  may: '5',
  jun: '6',
  jul: '7',
  aug: '8',
  sep: '9',
  oct: '10',
  nov: '11',
  dec: '12'
};
function substituteMonths(part) {
  return replaceFromHash(part, MONTHS);
}

const SPECIAL_STRING_SUBSTITUTIONS = {
  '@yearly':   ['0', '0', '1', '1', '*'],
  '@annually': ['0', '0', '1', '1', '*'],
  '@monthly':  ['0', '0', '1', '*', '*'],
  '@weekly':   ['0', '0', '*', '*', '0'],
  '@daily':    ['0', '0', '*', '*', '*'],
  '@midnight': ['0', '0', '*', '*', '*'],
  '@hourly':   ['0', '*', '*', '*', '*']
};

function substituteSpecialStrings(scheduleExpression) {
  const substitution = SPECIAL_STRING_SUBSTITUTIONS[scheduleExpression];
  return substitution !== undefined ? substitution : [scheduleExpression];
}

module.exports = function(scheduleExpression) {
  const originalParts = scheduleExpression.trim().split(/\s+/).filter(p => p);
  if (originalParts.length === 1 && originalParts[0] === '@reboot') {
    return {originalParts, parts: []};
  }
  const preppedParts = (originalParts.length === 1) ? substituteSpecialStrings(originalParts[0]) : originalParts;
  const parts = preppedParts.map((part, index) => {
    switch (index) {
    case 3: return substituteMonths(part);
    case 4: return substituteWeekdays(part);
    default: return part;
    }
  });

// if date or weekday starts with *, they're ANDed, otherwise ORed
// only looking at the first character regardless of rest of string, just like Vixie cron
// comment from Paul Vixie: "yes, it's bizarre. like many bizarre things, it's the standard."
  const daysAnded = (!!parts[2] && parts[2][0] === '*') || (!!parts[4] && parts[4][0] === '*');

  return {
    originalParts,
    parts,
    daysAnded
  };
};
