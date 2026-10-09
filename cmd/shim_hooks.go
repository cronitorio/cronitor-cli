package cmd

// Test seams. Production leaves them nil.
var (
	telemetryTestHook  func()
	shellShimAfterJob  func()
	shimAfterHandshake func()
)
