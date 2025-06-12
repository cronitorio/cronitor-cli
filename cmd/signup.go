package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type model struct {
	inputs     []textinput.Model
	focused    int
	submitted  bool
	submitting bool
	err        error
}

func initialModel() model {
	inputs := make([]textinput.Model, 3)

	for i := range inputs {
		t := textinput.New()
		switch i {
		case 0:
			t.Placeholder = "Full Name"
			t.Focus()
		case 1:
			t.Placeholder = "Email Address"
		case 2:
			t.Placeholder = "Password"
			t.EchoMode = textinput.EchoPassword
		}
		inputs[i] = t
	}

	return model{
		inputs:  inputs,
		focused: 0,
	}
}

var signupCmd = &cobra.Command{
	Use:   "signup",
	Short: "Sign up for a Cronitor account",
	Long:  `Create a new Cronitor account by providing your name, email, and password.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := tea.NewProgram(initialModel()).Start(); err != nil {
			fmt.Printf("Error running signup: %v\n", err)
			return
		}
	},
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			s := msg.String()
			if s == "tab" {
				m.focused = (m.focused + 1) % len(m.inputs)
			} else {
				m.focused = (m.focused - 1 + len(m.inputs)) % len(m.inputs)
			}
			cmds := make([]tea.Cmd, len(m.inputs))
			for i := range m.inputs {
				if i == m.focused {
					cmds[i] = m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, tea.Batch(cmds...)
		case "enter":
			m.err = nil

			if m.focused == len(m.inputs)-1 {
				allFilled := true
				for i, input := range m.inputs {
					if input.Value() == "" {
						allFilled = false
						break
					}
					if i == 1 && (!strings.Contains(input.Value(), "@") || len(input.Value()) < 5) {
						m.err = fmt.Errorf("please enter a valid email address")
						return m, nil
					}
					if i == 2 && len(input.Value()) < 8 {
						m.err = fmt.Errorf("password must be at least 8 characters")
						return m, nil
					}
				}

				if allFilled {

					color := color.New(color.FgGreen)
					color.Println("\n✔ Submitting...")

					api := lib.CronitorApi{
						UserAgent: "cronitor-cli",
					}

					if m.submitting {
						return m, nil
					}
					m.submitting = true
					resp, err := api.Signup(
						m.inputs[0].Value(),
						m.inputs[1].Value(),
						m.inputs[2].Value(),
					)
					m.submitting = false
					if err != nil {
						m.err = err
						return m, tea.Quit
					}

					viper.Set(varApiKey, resp.ApiKey)
					viper.Set(varPingApiKey, resp.PingApiKey)
					m.submitted = true

					if err := viper.WriteConfig(); err != nil {
						m.err = fmt.Errorf("%v\n\nYour API keys could not be saved. Try setting them with sudo:\nsudo cronitor configure --api-key %s --ping-api-key %s", err, resp.ApiKey, resp.PingApiKey)
					}

					return m, tea.Quit
				}
			}

			if m.inputs[m.focused].Value() == "" {
				m.err = fmt.Errorf("this is a required field")
				return m, nil
			}

			m.focused = (m.focused + 1) % len(m.inputs)
			cmds := make([]tea.Cmd, len(m.inputs))
			for i := range m.inputs {
				if i == m.focused {
					cmds[i] = m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, tea.Batch(cmds...)
		}
	}

	cmd := m.updateInputs(msg)
	return m, cmd
}

func (m *model) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m model) View() string {
	var s string
	s += color.GreenString("\nSign up for Cronitor\n\n")

	for i := range m.inputs {
		s += fmt.Sprintf("%s\n", m.inputs[i].View())
	}

	s += "\n✔ By signing up, you agree to our terms and conditions (https://cronitor.io/terms)\n"

	if m.err != nil {
		s += color.RedString(fmt.Sprintf("\nError: %v\n", m.err))
	}
	if m.submitted {
		s += color.GreenString("\n✔ Sign up complete. Run 'cronitor discover' to get started.\n\n")
	}
	return s
}

func init() {
	RootCmd.AddCommand(signupCmd)
}
