import normalize from './normalize';
import prenormalize from './prenormalize';
import nextDate from './nextDate';
import dateFormatter from './dateFormatter';
//
// Normalize the input
//
function replaceFromHash(string, hash) {
  const saveReplace = (text, searchText, replacement) => {
    const regex = new RegExp(`(^|[ ,-/])${searchText}($|[ ,-/])`, "gi");
    const fullReplacement = `$1${replacement}$2`;
    return text.replace(regex, fullReplacement).replace(regex, fullReplacement);
  };
  return Object.keys(hash).reduce((text, key) => saveReplace(text, key, hash[key]), string);
}

const MONTHS = {
  jan: "1",
  feb: "2",
  mar: "3",
  apr: "4",
  may: "5",
  jun: "6",
  jul: "7",
  aug: "8",
  sep: "9",
  oct: "10",
  nov: "11",
  dec: "12",
};

//
// Create Description
//

function ordinal(number) {
  const int = parseInt(number);
  switch (int > 20 ? int % 10 : int) {
    case 1:
      return `${number}st`;
    case 2:
      return `${number}nd`;
    case 3:
      return `${number}rd`;
    default:
      return `${number}th`;
  }
}

function join(list) {
  switch (list.length) {
    case 0:
      return "";
    case 1:
      return list[0];
    case 2:
      return `${list[0]} and ${list[1]}`;
    default:
      return `${list.slice(0, list.length - 1).join(", ")}, and ${list[list.length - 1]}`;
  }
}

function describeRange(possibleRange, unit, expansions, max) {
  const elements = possibleRange.match(/\d+|./g).map((element) => {
    const number = Number(element);
    return isNaN(number) ? element : number;
  });
  const from = elements[0];
  if (Number.isInteger(from)) {
    if (elements.length === 1) {
      return `${expansions[from] || from}`;
    } else if (elements.length === 3 && elements[1] === "/" && Number.isInteger(elements[2])) {
      return `every ${ordinal(elements[2])} ${unit} from ${expansions[from] || from} through ${expansions[max] || max}`;
    } else if (elements.length === 3 && elements[1] === "-" && Number.isInteger(elements[2]) && elements[2] >= from) {
      return `every ${unit} from ${expansions[from] || from} through ${expansions[elements[2]] || elements[2]}`;
    } else if (
      elements.length === 5 &&
      elements[1] === "-" &&
      Number.isInteger(elements[2]) &&
      elements[2] >= from &&
      elements[3] === "/" &&
      Number.isInteger(elements[4]) &&
      elements[4] >= 1
    ) {
      return `every ${ordinal(elements[4])} ${unit} from ${expansions[from] || from} through ${
        expansions[elements[2]] || elements[2]
      }`;
    }
  } else if (elements.length === 3 && elements[1] === "/" && Number.isInteger(elements[2]) && elements[0] === "*") {
    return `every ${ordinal(elements[2])} ${unit}`;
  }
  return ""; // shouldn't happen
}

function listItemDescription(item, unit, expansions, max) {
  if (item === "*") {
    return `every ${unit}`;
  } else {
    return describeRange(item, unit, expansions, max);
  }
}

function listDescription(listString, unit, expansions, max, unitIsObvious) {
  const list = listString.split(",");
  const prefix = unitIsObvious ? "" : `${unit} `;
  return `${prefix}${join(list.map((number) => listItemDescription(number, unit, expansions, max)))}`
    .replace(`every 1st`, `every`)
    .replace(`${unit} every`, `every`)
    .replace(`, ${unit}`, `, `)
    .replace(`, and ${unit}`, `, and `);
}

function minutesDescription(minutes) {
  return listDescription(minutes, "minute", {}, 59);
}

function hoursDescription(hours) {
  if (hours === "*") {
    return "";
  }
  return "past " + listDescription(hours, "hour", {}, 23);
}

function datesDescription(dates) {
  if (dates === "*") {
    return "";
  }
  return "on " + listDescription(dates, "day-of-month", {}, 31);
}

const MONTH_NAMES = [
  null,
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];

function monthsDescription(months) {
  if (months === "*") {
    return "";
  }
  return "in " + listDescription(months, "month", MONTH_NAMES, 12, true);
}

const WEEKDAY_NAMES = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"];

function weekdaysDescription(weekdays) {
  if (weekdays === "*") {
    return "";
  }
  return "on " + listDescription(weekdays, "day-of-week", WEEKDAY_NAMES, 7, true);
}

const SIMPLE_NUMBER = /^0*\d\d?$/;

function simpleTime(minutes, hours) {
  if (SIMPLE_NUMBER.test(minutes) && SIMPLE_NUMBER.test(hours)) {
    return [("0" + minutes).slice(-2), ("0" + hours).slice(-2)];
  }
  return null;
}

const REBOOT = "After rebooting.";

function describe(prenormalizedSchedule) {
  if (!prenormalizedSchedule || !prenormalizedSchedule.originalParts) {
    return {
      full: "Invalid schedule",
      special: "Invalid schedule"
    };
  }

  if (prenormalizedSchedule.originalParts[0] === "@reboot") {
    return {
      full: REBOOT,
      special: REBOOT,
    };
  }

  const parts = prenormalizedSchedule.parts || [];
  if (parts.length < 5) {
    return {
      full: "Invalid schedule",
      special: "Invalid schedule"
    };
  }

  const dateText = datesDescription(parts[2]);
  const monthText = monthsDescription(parts[3]);
  const weekdayText = weekdaysDescription(parts[4]);
  let dateAndWeekday = "";
  if (dateText && weekdayText) {
    dateAndWeekday = prenormalizedSchedule.daysAnded ? "if it's" : "and";
  }
  const timeDescription = simpleTime(parts[0], parts[1]);
  if (timeDescription) {
    return {
      start: "Runs at",
      minutes: timeDescription[0],
      hours: timeDescription[1],
      isTime: true,
      dates: dateText || null,
      datesWeekdays: dateAndWeekday || null,
      weekdays: weekdayText || null,
      months: monthText || null,
      end: "",
      full: `Runs at ${timeDescription[1]}:${timeDescription[0]} ${dateText} ${dateAndWeekday} ${weekdayText} ${monthText}`
        .replace(/ +/g, " ")
        .trim(),
    };
  }
  const minutesText = minutesDescription(parts[0]);
  const hourText = hoursDescription(parts[1]);
  return {
    start: "Runs at",
    minutes: minutesText || null,
    hours: hourText || null,
    dates: dateText || null,
    datesWeekdays: dateAndWeekday || null,
    weekdays: weekdayText || null,
    months: monthText || null,
    end: "",
    full: `Runs at ${minutesText} ${hourText} ${dateText} ${dateAndWeekday} ${weekdayText} ${monthText}`
      .replace(/ +/g, " ")
      .trim(),
  };
}

export function getNextExecutionTimes(expression, jobTimezone, count = 10) {
  const schedule = normalize(prenormalize(expression));
  const now = new Date(new Date().toLocaleString('en-US', { timeZone: jobTimezone })) // Pretend the local time is in the job timezone
  
  let times = [];
  let date = nextDate(
    schedule, 
    new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate(), now.getHours(), now.getMinutes(), now.getSeconds())) // now pretend the job time is UTC for easier interpretation of the results
  )

  
  if (date) {
    let dates = [date]
    while (--count > 0) {
      date = nextDate(schedule, new Date(date.getTime() + 1))
      dates.push(date)
    }

    times = dates.map((date) => {
      const formattedDate = dateFormatter(date, jobTimezone)
      return {
        job: `${formattedDate.job.year}-${formattedDate.job.month}-${formattedDate.job.date} ${formattedDate.job.hour}:${formattedDate.job.minute}:00`, 
        jobTimezone: formattedDate.job.zone,

        local: `${formattedDate.local.year}-${formattedDate.local.month}-${formattedDate.local.date} ${formattedDate.local.hour}:${formattedDate.local.minute}:00`,
        localTimezone: formattedDate.local.zone
      }
    })
  }
  return times


}

const guru = (cron, timezone) => {
  const details = describe(prenormalize(cron));
  const description = details.isTime
    ? `${details.start || ""} ${details.hours || ""}:${details.minutes || ""} ${timezone || ""} ${
        details.dates || ""
      } ${details.datesWeekdays || ""} ${details.weekdays || ""} ${details.months || ""} ${details.end || ""}`
    : `${details.start || ""} ${details.minutes || ""} ${details.hours || ""} ${details.dates || ""} ${
        details.datesWeekdays || ""
      } ${details.weekdays || ""} ${details.months || ""} ${details.end || ""}`;

  return description.replace("at every", "every");
};

export default guru;
