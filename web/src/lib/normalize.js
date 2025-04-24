function intSort(a, b) {
  return a - b;
}

function uniq(array) {
  // return Array.from(new Set(array)); // not working in IE11 even with polyfills when adding a comma to the cron schedule
  return array.reduce(function(p, c) {
    if (p.indexOf(c) < 0) p.push(c);
    return p;
  }, []);
}

function flatten(deepArray) {
  return deepArray.reduce((result, item) => result.concat(Array.isArray(item) ? flatten(item) : item), []);
}

function numbersInRange(from, to, step) {
  const result = [];
  for (let current = from; current <= to; current += step) {
    result.push(current);
  }
  return result;
}

function derange(possibleRange, max) {
  const elements = possibleRange ? possibleRange.match(/\d+|./g).map(element => {
    const number = Number(element);
    return isNaN(number) ? element : number;
  }) : [];
  const from = elements[0];
  if (Number.isInteger(from)) {
    if (elements.length === 1) {
      return {list: [from]};
    } else if (elements.length === 3 && elements[1] === '/' && Number.isInteger(elements[2]) && elements[2] >= 1) {
      return {list: numbersInRange(from, max, elements[2]), warnings: ['nonstandard']};
    } else if (elements.length === 3 && elements[1] === '-' && Number.isInteger(elements[2]) && elements[2] >= from) {
      return {list: numbersInRange(from, elements[2], 1)};
    } else if (elements.length === 5 && elements[1] === '-' && Number.isInteger(elements[2]) && elements[2] >= from
                                     && elements[3] === '/' && Number.isInteger(elements[4]) && elements[4] >= 1) {
      return {list: numbersInRange(from, elements[2], elements[4])};
    }
  }
  return {errors: ['invalid part']};
}

const WILDCARD_REGEX = /(^|[,-/])\*($|[,-/])/g;
function substituteWildcards(part, range) {
  const replacement = `$1${range}$2`;
  return part.replace(WILDCARD_REGEX, replacement).replace(WILDCARD_REGEX, replacement); // twice due to potentially overlapping separators such as in '*,*'
}

function normalizeWeekday7(numbers) {
  return uniq(numbers.map(number => number === 7 ? 0 : number)).sort(intSort);
}

function normalizePart(part, max) {
  const rangeResults = part.split(',').map(possibleRange => derange(possibleRange, max));
  const list = uniq(flatten(rangeResults.map(rangeResult => rangeResult.list || []))).sort(intSort).filter(number => !isNaN(number));
  const errors = uniq(flatten(rangeResults.map(rangeResult => rangeResult.errors || [])));
  const warnings = uniq(flatten(rangeResults.map(rangeResult => rangeResult.warnings || [])));
  return {list, errors, warnings};
}

function tooHighOrLow(sortedNumbers, min, max) {
  return sortedNumbers.length && (sortedNumbers[0] < min || sortedNumbers[sortedNumbers.length - 1] > max);
}

const INVALID_PART_REGEX = /[^\d\-\/\,]/i;

function normalizeSingleSteps(parts) {
  // '*/1 */6,*/1,11-13/11 * */3 */1' => '* */6,*,11-13/11 * */3 *'
  return parts.map(part => part.replace(/\*\/1(?!\d)/g, '*'));
}

module.exports = function(prenormalizedSchedule) {
  const parts = normalizeSingleSteps(prenormalizedSchedule.parts.map(part => part.slice(0))); // makes deep copy to avoid modifying parameter
  if (parts.length === 0 && prenormalizedSchedule.originalParts.length) {
    return {};
  }
  const schedule = {
    errors: [],
    warnings: []
  };
  if (prenormalizedSchedule.daysAnded !== undefined) {
    schedule.daysAnded = prenormalizedSchedule.daysAnded;
  }
  if (parts.length !== 5) {
    schedule.errors.push('fields');
  }

  if (parts[0] && parts[0].length) {
    const part = substituteWildcards(parts[0], '0-59');
    const result = normalizePart(part, 59);
    schedule.minutes = result.list;
    if (result.errors.length || tooHighOrLow(schedule.minutes, 0, 59) || INVALID_PART_REGEX.test(part)) {
      schedule.minutes = [];
      schedule.errors.push('minutes');
    }
    if (result.warnings.length) {
      schedule.warnings.push('minutes');
    }
  } else if (parts[0] === undefined) {
    schedule.errors.push('minutes');
  }
  if (parts[1] && parts[1].length) {
    const part = substituteWildcards(parts[1], '0-23');
    const result = normalizePart(part, 23);
    schedule.hours = result.list;
    if (result.errors.length || tooHighOrLow(schedule.hours, 0, 23) || INVALID_PART_REGEX.test(part)) {
      schedule.hours = [];
      schedule.errors.push('hours');
    }
    if (result.warnings.length) {
      schedule.warnings.push('hours');
    }
  } else if (parts[1] === undefined) {
    schedule.errors.push('hours');
  }
  if (parts[2] && parts[2].length) {
    const part = substituteWildcards(parts[2], '1-31');
    const result = normalizePart(part, 31);
    schedule.dates = result.list;
    if (result.errors.length || tooHighOrLow(schedule.dates, 1, 31) || INVALID_PART_REGEX.test(part)) {
      schedule.dates = [];
      schedule.errors.push('dates');
    }
    if (result.warnings.length) {
      schedule.warnings.push('dates');
    }
  } else if (parts[2] === undefined) {
    schedule.errors.push('dates');
  }
  if (parts[3] && parts[3].length) {
    const part = substituteWildcards(parts[3], '1-12');
    const originalPart = prenormalizedSchedule.originalParts[3];
    const result = normalizePart(part, 12);
    schedule.months = result.list;
    if (result.errors.length || tooHighOrLow(schedule.months, 1, 12) || INVALID_PART_REGEX.test(part)) {
      schedule.months = [];
      schedule.errors.push('months');
    }
    if (result.warnings.length || (originalPart && parts[3] !== originalPart && originalPart.length > 3 && /\D/.test(originalPart))) {
      schedule.warnings.push('months');
    }
  } else if (parts[3] === undefined) {
    schedule.errors.push('months');
  }
  if (parts[4] && parts[4].length) {
    const part = substituteWildcards(parts[4], '0-6');
    const originalPart = prenormalizedSchedule.originalParts[4];
    const result = normalizePart(part, 7);
    schedule.weekdays = normalizeWeekday7(result.list);
    if (result.errors.length || tooHighOrLow(schedule.weekdays, 0, 6) || INVALID_PART_REGEX.test(part)) {
      schedule.weekdays = [];
      schedule.errors.push('weekdays');
    }
    if (result.warnings.length || result.list.includes(7) || (originalPart && parts[4] !== originalPart && originalPart.length > 3 && /\D/.test(originalPart))) {
      schedule.warnings.push('weekdays');
    }
  } else if (parts[4] === undefined) {
    schedule.errors.push('weekdays');
  }
  if (!schedule.errors.length) {
    delete schedule.errors;
  }
  if (!schedule.warnings.length) {
    delete schedule.warnings;
  }
  return schedule;
};
