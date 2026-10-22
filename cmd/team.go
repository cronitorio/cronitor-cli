package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var teamCmd = &cobra.Command{
	Use:     "team",
	Aliases: []string{"teams"},
	Short:   "Manage team members",
	Long: `Manage the members of your Cronitor team.

Team commands act as the user who created the API key, with that user's
current role. They need a key with account access: run
'cronitor auth login --admin' on a machine you trust. Any role can list the
team; inviting, changing roles, and removing members need the admin role.

Roles are admin, user, and readonly. Team ownership can't be transferred
from the CLI.

Examples:
  cronitor team list
  cronitor team list --format json
  cronitor team invite jane@example.com --name "Jane Doe"
  cronitor team invite ops@example.com --name "Ops" --role readonly
  cronitor team role jane@example.com admin
  cronitor team remove jane@example.com
  cronitor team remove jane@example.com --yes`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("API key required. Run 'cronitor configure' or use --api-key flag")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var teamRoles = []string{"admin", "user", "readonly"}

var (
	teamFormat string
	teamOutput string
	teamName   string
	teamRole   string
	teamYes    bool
)

func init() {
	RootCmd.AddCommand(teamCmd)
	teamCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if err := validateOutputFormat(teamFormat); err != nil {
			failAndExit(err.Error())
		}
	}
	teamCmd.PersistentFlags().StringVar(&teamFormat, "format", "", "Output format: json, table")
	teamCmd.PersistentFlags().StringVarP(&teamOutput, "output", "o", "", "Write output to file")
}

func resetTeamFlags() {
	teamFormat = ""
	teamOutput = ""
	teamName = ""
	teamRole = "user"
	teamYes = false
}

type teamMember struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	State   string `json:"state"`
	IsOwner bool   `json:"is_owner"`
}

// --- LIST ---
var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List team members and pending invites",
	Long: `List team members and pending invites.

Examples:
  cronitor team list
  cronitor team list --format json`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET("/team", nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to list team members: %s", err))
			return nil
		}

		if !resp.IsSuccess() {
			failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			return nil
		}

		var result struct {
			Data []teamMember `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			return fmt.Errorf("Failed to parse response: %s", err)
		}

		format := teamFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			writeCLIOutput(teamOutput, FormatJSON(resp.Body))
			return nil
		}

		table := &UITable{
			Headers: []string{"ID", "NAME", "EMAIL", "ROLE", "STATE", "OWNER"},
		}

		for _, m := range result.Data {
			owner := ""
			if m.IsOwner {
				owner = "yes"
			}
			table.Rows = append(table.Rows, []string{m.ID, m.Name, m.Email, m.Role, m.State, owner})
		}

		writeCLIOutput(teamOutput, table.Render())
		return nil
	},
}

// --- INVITE ---
var teamInviteCmd = &cobra.Command{
	Use:   "invite <email>",
	Short: "Invite someone to the team",
	Long: `Invite someone to the team. They join as a pending member until they accept.

The role defaults to user. Inviting needs the admin role.

Examples:
  cronitor team invite jane@example.com --name "Jane Doe"
  cronitor team invite ops@example.com --name "Ops" --role readonly`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		email := args[0]

		if err := validateTeamMemberID(email); err != nil {
			failAndExit(err.Error())
			return
		}
		if strings.TrimSpace(teamName) == "" {
			failAndExit("Name required. Use --name")
			return
		}
		if err := validateTeamRole(teamRole); err != nil {
			failAndExit(err.Error())
			return
		}

		body, _ := json.Marshal(map[string]string{"email": email, "name": teamName, "role": teamRole})

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/team", body, nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to invite team member: %s", err))
			return
		}

		if !resp.IsSuccess() {
			failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			return
		}

		successForFormat(teamFormat, fmt.Sprintf("Invited %s as %s", email, teamRole))
		if teamFormat == "json" {
			writeCLIOutput(teamOutput, FormatJSON(resp.Body))
		}
	},
}

// --- ROLE ---
var teamRoleCmd = &cobra.Command{
	Use:   "role <email> <admin|user|readonly>",
	Short: "Change a team member's role",
	Long: `Change a team member's role. The member id is their email address.
Find it with 'cronitor team list'.

Changing roles needs the admin role. Team ownership can't be transferred
from the CLI.

Examples:
  cronitor team role jane@example.com admin
  cronitor team role jane@example.com readonly`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		id, role := args[0], args[1]

		if err := validateTeamMemberID(id); err != nil {
			failAndExit(err.Error())
			return
		}
		if err := validateTeamRole(role); err != nil {
			failAndExit(err.Error())
			return
		}

		body, _ := json.Marshal(map[string]string{"role": role})

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(teamMemberPath(id), body, nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to change role: %s", err))
			return
		}

		if resp.IsNotFound() {
			failAndExit(fmt.Sprintf("Team member '%s' not found", id))
			return
		}

		if !resp.IsSuccess() {
			failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			return
		}

		successForFormat(teamFormat, fmt.Sprintf("Team member '%s' is now %s", id, role))
		if teamFormat == "json" {
			writeCLIOutput(teamOutput, FormatJSON(resp.Body))
		}
	},
}

// --- REMOVE ---
var teamRemoveCmd = &cobra.Command{
	Use:   "remove <email>",
	Short: "Remove a team member or cancel an invite",
	Long: `Remove a team member, or cancel a pending invite. Their API keys stop
working at once. The member id is their email address. Find it with
'cronitor team list'.

Asks for confirmation unless --yes is set. Removing needs the admin role.

Examples:
  cronitor team remove jane@example.com
  cronitor team remove jane@example.com --yes`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]

		if err := validateTeamMemberID(id); err != nil {
			failAndExit(err.Error())
			return
		}
		if err := confirmAccountChange(fmt.Sprintf("Remove team member %s?", id), teamYes); err != nil {
			failAndExit(fmt.Sprintf("Team member '%s' not removed: %s", id, err))
			return
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.DELETE(teamMemberPath(id), nil, nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to remove team member: %s", err))
			return
		}

		if resp.IsNotFound() {
			failAndExit(fmt.Sprintf("Team member '%s' not found", id))
			return
		}

		if !resp.IsSuccess() {
			failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			return
		}

		successForFormat(teamFormat, fmt.Sprintf("Team member '%s' removed", id))
	},
}

func init() {
	teamCmd.AddCommand(teamListCmd)
	teamCmd.AddCommand(teamInviteCmd)
	teamCmd.AddCommand(teamRoleCmd)
	teamCmd.AddCommand(teamRemoveCmd)

	// Invite flags
	teamInviteCmd.Flags().StringVar(&teamName, "name", "", "Name of the person to invite")
	teamInviteCmd.Flags().StringVar(&teamRole, "role", "user", "Role: admin, user, readonly")

	// Remove flags
	teamRemoveCmd.Flags().BoolVar(&teamYes, "yes", false, "Remove without prompting")
}

func validateTeamRole(role string) error {
	for _, r := range teamRoles {
		if role == r {
			return nil
		}
	}
	return fmt.Errorf("Invalid role '%s'. Use one of: %s", role, strings.Join(teamRoles, ", "))
}

// teamMemberPath is /team/<id> with the id escaped as one path segment.
// The id is an opaque string (the member email, in whatever casing the
// caller passed). Django 4.2 decodes that segment once: a literal '+' and
// '%2B' both mean '+', and '%40' means '@'. PathEscape leaves '+' and '@'
// literal, which matches that single decode. An encoded or literal slash,
// or a trailing slash, does not match the route.
func teamMemberPath(email string) string {
	return "/team/" + url.PathEscape(email)
}

// validateTeamMemberID accepts the email the API returns as id. Invite,
// role, and remove all use it. The value is kept as given: it is not
// case-folded or rewritten. Numeric database ids are not public
// identifiers. A slash can never match the server route
// re_path(r'^api/team/(?P<email>[^/]+)$'), so it is rejected here.
func validateTeamMemberID(id string) error {
	if strings.Contains(id, "/") {
		return fmt.Errorf("Invalid team member id '%s'. Emails containing '/' can't be managed from the CLI", id)
	}
	addr, err := mail.ParseAddress(id)
	if err != nil || addr.Address != id {
		return fmt.Errorf("Invalid team member id '%s'. Use the email from 'cronitor team list'", id)
	}
	return nil
}
