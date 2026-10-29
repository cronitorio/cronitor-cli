package lib_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
)

const pySimple = `Starting nightly export
Traceback (most recent call last):
  File "/app/export.py", line 42, in <module>
    main()
  File "/app/export.py", line 37, in main
    rows = fetch_rows(conn)
           ^^^^^^^^^^^^^^^^
  File "/app/db.py", line 15, in fetch_rows
    raise ValueError("boom")
ValueError: boom
`

const pyChained = `Traceback (most recent call last):
  File "/app/sync.py", line 10, in load
    return json.loads(raw)
           ^^^^^^^^^^^^^^^
  File "/usr/lib/python3.11/json/__init__.py", line 346, in loads
    return _default_decoder.decode(s)
           ^^^^^^^^^^^^^^^^^^^^^^^^^^
json.decoder.JSONDecodeError: Expecting value: line 1 column 1 (char 0)

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "/app/sync.py", line 20, in <module>
    load()
  File "/app/sync.py", line 12, in load
    raise RuntimeError("could not parse payload")
RuntimeError: could not parse payload
`

const pyDirectCause = `Traceback (most recent call last):
  File "/app/fetch.py", line 4, in get
    return urlopen(url)
urllib.error.URLError: <urlopen error [Errno 111] Connection refused>

The above exception was the direct cause of the following exception:

Traceback (most recent call last):
  File "/app/fetch.py", line 9, in <module>
    get("http://localhost")
  File "/app/fetch.py", line 6, in get
    raise FetchError("upstream down") from e
__main__.FetchError: upstream down
`

const pyInNoise = `2026-10-29 03:00:01 INFO starting backup
2026-10-29 03:00:02 INFO connecting to s3
ERROR:root:upload failed
Traceback (most recent call last):
  File "/opt/backup/run.py", line 88, in upload
    client.put_object(Bucket=bucket, Key=key, Body=body)
  File "/opt/venv/lib/python3.12/site-packages/botocore/client.py", line 565, in _api_call
    return self._make_api_call(operation_name, kwargs)
           ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
botocore.exceptions.ClientError: An error occurred (AccessDenied) when calling the PutObject operation: Access Denied
2026-10-29 03:00:03 INFO cleanup done
exiting with status 1
`

const pyKeyboardInterrupt = `tick
tick
^CTraceback (most recent call last):
  File "/app/loop.py", line 5, in <module>
    time.sleep(1)
KeyboardInterrupt
`

const pySyntaxError = `Traceback (most recent call last):
  File "/app/main.py", line 1, in <module>
    import broken
  File "/app/broken.py", line 3
    def main(
            ^
SyntaxError: '(' was never closed
`

const pyRecursion = `Traceback (most recent call last):
  File "/app/r.py", line 4, in <module>
    f()
  File "/app/r.py", line 2, in f
    return f()
           ^^^
  [Previous line repeated 996 more times]
RecursionError: maximum recursion depth exceeded
`

const pyTruncated = `Traceback (most recent call last):
  File "/app/job.py", line 3, in <module>
    main()
`

const nodeUncaught = `/app/job.js:12
    foo.bar();
        ^

TypeError: foo.bar is not a function
    at main (/app/job.js:12:9)
    at Object.<anonymous> (/app/job.js:20:1)
    at Module._compile (node:internal/modules/cjs/loader:1256:14)
    at node:internal/main/run_main_module:23:47

Node.js v18.19.0
`

const nodeRejection = `connecting to database
node:internal/process/promises:288
            triggerUncaughtException(err, true /* fromPromise */);
            ^

Error: connect ECONNREFUSED 127.0.0.1:5432
    at TCPConnectWrap.afterConnect [as oncomplete] (node:net:1494:16)
    at async Pool.connect (/app/node_modules/pg-pool/index.js:45:11)
    at async run (file:///app/sync.mjs:8:3) {
  errno: -111,
  code: 'ECONNREFUSED',
  syscall: 'connect',
  address: '127.0.0.1',
  port: 5432
}

Node.js v20.11.1
`

const javaTrace = `Exception in thread "main" java.lang.IllegalStateException: boom
	at com.example.Job.run(Job.java:42)
	at com.example.Main.main(Main.java:10)
`

func TestTracebackParse(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   *lib.ParsedError
	}{
		{
			name:   "python simple with caret lines",
			output: pySimple,
			want: &lib.ParsedError{Type: "ValueError", Value: "boom", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/export.py", Function: "<module>", Lineno: 42, ContextLine: "main()", InApp: true},
				{Filename: "/app/export.py", Function: "main", Lineno: 37, ContextLine: "rows = fetch_rows(conn)", InApp: true},
				{Filename: "/app/db.py", Function: "fetch_rows", Lineno: 15, ContextLine: `raise ValueError("boom")`, InApp: true},
			}},
		},
		{
			name:   "python chained during handling takes the last block",
			output: pyChained,
			want: &lib.ParsedError{Type: "RuntimeError", Value: "could not parse payload", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/sync.py", Function: "<module>", Lineno: 20, ContextLine: "load()", InApp: true},
				{Filename: "/app/sync.py", Function: "load", Lineno: 12, ContextLine: `raise RuntimeError("could not parse payload")`, InApp: true},
			}},
		},
		{
			name:   "python direct cause with dotted type",
			output: pyDirectCause,
			want: &lib.ParsedError{Type: "__main__.FetchError", Value: "upstream down", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/fetch.py", Function: "<module>", Lineno: 9, ContextLine: `get("http://localhost")`, InApp: true},
				{Filename: "/app/fetch.py", Function: "get", Lineno: 6, ContextLine: `raise FetchError("upstream down") from e`, InApp: true},
			}},
		},
		{
			name:   "python embedded in log noise, library frame not in app",
			output: pyInNoise,
			want: &lib.ParsedError{
				Type:     "botocore.exceptions.ClientError",
				Value:    "An error occurred (AccessDenied) when calling the PutObject operation: Access Denied",
				Platform: "python",
				Frames: []lib.Frame{
					{Filename: "/opt/backup/run.py", Function: "upload", Lineno: 88, ContextLine: "client.put_object(Bucket=bucket, Key=key, Body=body)", InApp: true},
					{Filename: "/opt/venv/lib/python3.12/site-packages/botocore/client.py", Function: "_api_call", Lineno: 565, ContextLine: "return self._make_api_call(operation_name, kwargs)", InApp: false},
				},
			},
		},
		{
			name:   "python bare exception type after a prefixed header",
			output: pyKeyboardInterrupt,
			want: &lib.ParsedError{Type: "KeyboardInterrupt", Value: "", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/loop.py", Function: "<module>", Lineno: 5, ContextLine: "time.sleep(1)", InApp: true},
			}},
		},
		{
			name:   "python syntax error frame without function",
			output: pySyntaxError,
			want: &lib.ParsedError{Type: "SyntaxError", Value: "'(' was never closed", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/main.py", Function: "<module>", Lineno: 1, ContextLine: "import broken", InApp: true},
				{Filename: "/app/broken.py", Lineno: 3, ContextLine: "def main(", InApp: true},
			}},
		},
		{
			name:   "python recursion with repeated-line marker",
			output: pyRecursion,
			want: &lib.ParsedError{Type: "RecursionError", Value: "maximum recursion depth exceeded", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/r.py", Function: "<module>", Lineno: 4, ContextLine: "f()", InApp: true},
				{Filename: "/app/r.py", Function: "f", Lineno: 2, ContextLine: "return f()", InApp: true},
			}},
		},
		{
			name:   "python with windows line endings",
			output: strings.ReplaceAll(pySimple, "\n", "\r\n"),
			want: &lib.ParsedError{Type: "ValueError", Value: "boom", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/export.py", Function: "<module>", Lineno: 42, ContextLine: "main()", InApp: true},
				{Filename: "/app/export.py", Function: "main", Lineno: 37, ContextLine: "rows = fetch_rows(conn)", InApp: true},
				{Filename: "/app/db.py", Function: "fetch_rows", Lineno: 15, ContextLine: `raise ValueError("boom")`, InApp: true},
			}},
		},
		{
			name:   "node uncaught exception, frames reversed to oldest first",
			output: nodeUncaught,
			want: &lib.ParsedError{Type: "TypeError", Value: "foo.bar is not a function", Platform: "node", Frames: []lib.Frame{
				{Filename: "node:internal/main/run_main_module", Lineno: 23, Colno: 47, InApp: false},
				{Filename: "node:internal/modules/cjs/loader", Function: "Module._compile", Lineno: 1256, Colno: 14, InApp: false},
				{Filename: "/app/job.js", Function: "Object.<anonymous>", Lineno: 20, Colno: 1, InApp: true},
				{Filename: "/app/job.js", Function: "main", Lineno: 12, Colno: 9, InApp: true},
			}},
		},
		{
			name:   "node unhandled rejection with async frames and error properties",
			output: nodeRejection,
			want: &lib.ParsedError{Type: "Error", Value: "connect ECONNREFUSED 127.0.0.1:5432", Platform: "node", Frames: []lib.Frame{
				{Filename: "file:///app/sync.mjs", Function: "run", Lineno: 8, Colno: 3, InApp: true},
				{Filename: "/app/node_modules/pg-pool/index.js", Function: "Pool.connect", Lineno: 45, Colno: 11, InApp: false},
				{Filename: "node:net", Function: "TCPConnectWrap.afterConnect [as oncomplete]", Lineno: 1494, Colno: 16, InApp: false},
			}},
		},
		{
			name: "python with a timestamp prefix on every line",
			output: "2026-10-29T03:00:00Z starting\n" +
				"2026-10-29T03:00:01Z Traceback (most recent call last):\n" +
				"2026-10-29T03:00:01Z   File \"/app/job.py\", line 3, in <module>\n" +
				"2026-10-29T03:00:01Z     main()\n" +
				"2026-10-29T03:00:01Z ValueError: boom\n",
			want: &lib.ParsedError{Type: "ValueError", Value: "boom", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/job.py", Function: "<module>", Lineno: 3, ContextLine: "main()", InApp: true},
			}},
		},
		{
			name: "python with a word prefix that looks like an exception type",
			output: "job: Traceback (most recent call last):\n" +
				"job:   File \"/app/job.py\", line 3, in <module>\n" +
				"job:     main()\n" +
				"job:   File \"/app/job.py\", line 1, in main\n" +
				"job:     raise KeyError(\"x\")\n" +
				"job: KeyError: 'x'\n",
			want: &lib.ParsedError{Type: "KeyError", Value: "'x'", Platform: "python", Frames: []lib.Frame{
				{Filename: "/app/job.py", Function: "<module>", Lineno: 3, ContextLine: "main()", InApp: true},
				{Filename: "/app/job.py", Function: "main", Lineno: 1, ContextLine: `raise KeyError("x")`, InApp: true},
			}},
		},
		{name: "plain shell failure", output: "rsync: connection refused\nrsync error: some files could not be transferred\n", want: nil},
		{name: "java stack trace is not recognized", output: javaTrace, want: nil},
		{name: "truncated python traceback without exception line", output: pyTruncated, want: nil},
		{name: "empty output", output: "", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := lib.ParseTraceback(tc.output)
			if tc.want == nil {
				if ok {
					t.Fatalf("want no traceback, got %+v", got)
				}
				return
			}
			if !ok {
				t.Fatal("traceback not recognized")
			}
			if got.Type != tc.want.Type || got.Value != tc.want.Value || got.Platform != tc.want.Platform {
				t.Errorf("got %q: %q (%s), want %q: %q (%s)", got.Type, got.Value, got.Platform, tc.want.Type, tc.want.Value, tc.want.Platform)
			}
			if !reflect.DeepEqual(got.Frames, tc.want.Frames) {
				t.Errorf("frames\n got: %+v\nwant: %+v", got.Frames, tc.want.Frames)
			}
		})
	}
}

func TestTracebackLastOfPythonAndNodeWins(t *testing.T) {
	got, ok := lib.ParseTraceback(nodeUncaught + "retrying with the python worker\n" + pySimple)
	if !ok || got.Platform != "python" || got.Type != "ValueError" {
		t.Fatalf("want the later python traceback, got %+v", got)
	}
	got, ok = lib.ParseTraceback(pySimple + "falling back to node\n" + nodeUncaught)
	if !ok || got.Platform != "node" || got.Type != "TypeError" {
		t.Fatalf("want the later node stack, got %+v", got)
	}
}

func TestTracebackTruncatedLastBlockFallsBackToEarlierBlock(t *testing.T) {
	got, ok := lib.ParseTraceback(pySimple + pyTruncated)
	if !ok || got.Type != "ValueError" {
		t.Fatalf("want the complete earlier block, got %+v ok=%v", got, ok)
	}
}
