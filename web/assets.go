// Package web holds the dashboard's built frontend. Run `npm run build` in
// this folder to write web/static before building crontab-dashboard.
package web

import "embed"

//go:embed static
var Static embed.FS
