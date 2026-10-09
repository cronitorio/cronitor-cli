package cmd

var (
	telemetryTestHook  func()
	shellShimAfterJob  func()
	shimAfterHandshake func()
	telemetryDoneHook  func()
)

func finishTelemetry() {
	if telemetryDoneHook != nil {
		telemetryDoneHook()
	}
}
