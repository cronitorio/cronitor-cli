# Dashboard development

Use Node.js 22.12 or newer (CI uses Node 22). Puppeteer 25 requires this runtime;
older Puppeteer versions pull in the unpatched `extract-zip` dependency.

```sh
npm ci
npm run test:dependencies
npm run build
```

`npm run screenshots` uses Puppeteer to capture a local dashboard on port 9000.
Browser installation requires `unzip` on Linux/macOS or `tar.exe` on Windows.
If browser downloads are unnecessary for a build-only environment, install with
`PUPPETEER_SKIP_DOWNLOAD=true npm ci`.

The version-scoped js-yaml overrides preserve the separate v3 and v4 APIs used
by the build tools. The dependency tests guard their patched versions, the SVGO
override, and the absence of extract-zip. These checks cover those dependencies,
not every advisory in the wider Create React App dependency tree.
