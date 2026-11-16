`npm run build` writes the dashboard frontend here. Go embeds this folder
into `crontab-dashboard` (see `web/assets.go`). This file is committed so Go
can build and test without Node; a build without the frontend serves no UI.
