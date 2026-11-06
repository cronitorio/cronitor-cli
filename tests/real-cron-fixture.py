#!/usr/bin/env python3
"""Helpers for tests/real-cron.sh: run every fixture job shape under real cron.

  real-cron-fixture.py lines fixtures/shim/crontab.input
      Print each fixture job line with an every-minute schedule.

  real-cron-fixture.py check <synced crontab> <requests.log> fixtures/shim/crontab.outputs
      For each CRONITOR=/MONITORIO= line whose command is in crontab.outputs,
      check that real cron sent one run ping and one final ping, with the
      job's exit code and output (empty with CRONITOR_NO_STDOUT=1).
      crontab.outputs is written by TestShimFixtureRuns from the original lines.
"""
import json
import re
import sys
from urllib.parse import parse_qs, urlparse

SETTINGS = {"CRONITOR_NO_STDOUT", "CRONITOR_ENV", "CRONITOR_API_KEY", "CRONITOR_PING_API_KEY", "CRONITOR_HOSTNAME"}
SIMPLE = re.compile(r"^[A-Za-z0-9._:@/+-]+$")
WS = " \t\n\v\f\r"


def job_field(line):
    """The command field of a job line, or None for env, comment and blank lines."""
    s = line.strip()
    if not s or s.startswith("#"):
        return None
    words = s.split(None, 1)
    if s.startswith("@"):
        return words[1] if len(words) > 1 else None
    if "=" in words[0]:
        return None
    parts = s.split(None, 5)
    return parts[5] if len(parts) == 6 else None


def split_marker(field):
    """Same rules as lib.ParseMonitorMarker and lib.SplitShimSettings."""
    for prefix in ("CRONITOR=", "MONITORIO="):
        if field.startswith(prefix):
            body = field[len(prefix):]
            break
    else:
        return None
    i = 0
    while i < len(body) and body[i] not in WS:
        i += 1
    if i == 0 or i == len(body):
        return None
    key, rest = body[:i], body[i:].lstrip(WS)
    settings = []
    while True:
        j = 0
        while j < len(rest) and rest[j] not in WS:
            j += 1
        word, after = rest[:j], rest[j:].lstrip(WS)
        name, eq, value = word.partition("=")
        if not eq or name not in SETTINGS or not SIMPLE.match(value) or not after:
            break
        settings.append(word)
        rest = after
    return key, settings, rest


def lines(path):
    for line in open(path):
        if job_field(line) is None:
            continue
        s = line.strip()
        if s.startswith("@"):
            print("* * * * * " + s.split(None, 1)[1])
        else:
            print("* * * * * " + s.split(None, 5)[5])


def pings_for(requests, key):
    found = []
    for entry in requests:
        if not entry.startswith("GET "):
            continue
        url = urlparse(entry[4:].strip())
        q = parse_qs(url.query, keep_blank_values=True)
        state = None
        if "state" in q and url.path.endswith("/" + key):
            state = q["state"][0]
        elif url.path.rsplit("/", 2)[-2:-1] == [key]:
            state = url.path.rsplit("/", 1)[1]
        if state:
            found.append((state, q.get("msg", [""])[0], q.get("status_code", [""])[0]))
    return found


def check(crontab, requests_log, outputs_path):
    outputs = json.load(open(outputs_path))
    requests = open(requests_log).read().splitlines()
    failed = checked = 0
    for line in open(crontab):
        field = job_field(line)
        if field is None:
            continue
        parsed = split_marker(field)
        if parsed is None:
            continue
        key, settings, job = parsed
        if job not in outputs:
            continue
        checked += 1
        want = outputs[job]
        no_stdout = "CRONITOR_NO_STDOUT=1" in settings
        pings = pings_for(requests, key)
        runs = [p for p in pings if p[0] == "run"]
        finals = [p for p in pings if p[0] in ("complete", "fail")]
        problem = None
        if len(runs) != 1 or len(finals) != 1:
            problem = "pings: %d run, %d final" % (len(runs), len(finals))
        else:
            state, msg, status = finals[0]
            if status != str(want["exit"]):
                problem = "exit %s, want %d" % (status, want["exit"])
            elif no_stdout and msg != "":
                problem = "output sent despite CRONITOR_NO_STDOUT=1: %r" % msg
            elif not no_stdout:
                expected = want["output"].strip() if want["exit"] == 0 else \
                    (want["output"] + " [exit status %d]" % want["exit"]).strip()
                if msg.strip() != expected:
                    problem = "output %r, want %r" % (msg, expected)
        if problem:
            failed += 1
            print("FAIL fixture %s (%s): %s" % (key, job, problem))
        else:
            print("ok   fixture %s (%s)" % (key, job))
    if checked < 20:
        print("FAIL only %d fixture jobs checked" % checked)
        failed += 1
    return 1 if failed else 0


if __name__ == "__main__":
    if sys.argv[1:2] == ["lines"] and len(sys.argv) == 3:
        lines(sys.argv[2])
    elif sys.argv[1:2] == ["check"] and len(sys.argv) == 5:
        sys.exit(check(*sys.argv[2:]))
    else:
        sys.exit(__doc__)
