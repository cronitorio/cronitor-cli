package cmd

import (
	"embed"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed web/dist
var webAssets embed.FS

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Start the Cronitor web dashboard",
	Long: `Start the Cronitor web dashboard server.
The dashboard provides a web interface for managing your Cronitor monitors and crontabs.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		if port == 0 {
			port = 9000
		}

		// Basic auth middleware
		authMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username := viper.GetString(varDashUsername)
				password := viper.GetString(varDashPassword)

				if username == "" || password == "" {
					http.Error(w, "Dashboard credentials not configured", http.StatusInternalServerError)
					return
				}

				auth := r.Header.Get("Authorization")
				if auth == "" {
					w.Header().Set("WWW-Authenticate", `Basic realm="Cronitor Dashboard"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				payload, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
				pair := strings.SplitN(string(payload), ":", 2)

				if len(pair) != 2 || pair[0] != username || pair[1] != password {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				next.ServeHTTP(w, r)
			})
		}

		// Get the embedded filesystem
		fsys, err := fs.Sub(webAssets, "web/dist")
		if err != nil {
			fatal(err.Error(), 1)
		}

		// Create a custom file server that serves index.html for all routes
		fileServer := http.FileServer(http.FS(fsys))
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to serve the requested file
			fileServer.ServeHTTP(w, r)

			// If the file wasn't found, serve index.html
			if w.Header().Get("Content-Type") == "" {
				index, err := fsys.Open("index.html")
				if err != nil {
					http.Error(w, "Not Found", http.StatusNotFound)
					return
				}
				defer index.Close()
				http.ServeContent(w, r, "index.html", time.Now(), index.(io.ReadSeeker))
			}
		})

		// Apply auth middleware to all routes
		http.Handle("/", authMiddleware(handler))

		fmt.Printf("Starting Cronitor dashboard on port %d...\n", port)
		if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
			fatal(err.Error(), 1)
		}
	},
}

func init() {
	RootCmd.AddCommand(dashCmd)
	dashCmd.Flags().Int("port", 9000, "Port to run the dashboard on")
}
