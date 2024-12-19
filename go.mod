module github.com/cronitorio/cronitor-cli

go 1.14

require (
	github.com/capnspacehook/taskmaster v0.0.0-20210519235353-1629df7c85e9
	github.com/certifi/gocertifi v0.0.0-20200211180108-c7c1fbc02894 // indirect
	github.com/fatih/color v1.9.0
	github.com/getsentry/raven-go v0.2.0
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/manifoldco/promptui v0.8.0
	github.com/olekukonko/tablewriter v0.0.4
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.9.0
)

require (
	github.com/charmbracelet/bubbles v0.20.0 // indirect
	github.com/charmbracelet/bubbletea v1.2.4 // indirect
	github.com/charmbracelet/lipgloss v1.0.0 // indirect
	github.com/pkg/errors v0.8.1
)

replace github.com/manifoldco/promptui => github.com/1lann/promptui v0.8.1-0.20201231190244-d8f2159af2b2
