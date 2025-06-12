const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

// Helper function to remove directory recursively
function removeDir(dirPath) {
  if (fs.existsSync(dirPath)) {
    fs.readdirSync(dirPath).forEach((file) => {
      const curPath = path.join(dirPath, file);
      if (fs.lstatSync(curPath).isDirectory()) {
        removeDir(curPath);
      } else {
        fs.unlinkSync(curPath);
      }
    });
    fs.rmdirSync(dirPath);
  }
}

// Helper function to copy directory recursively
function copyDir(src, dest) {
  if (!fs.existsSync(dest)) {
    fs.mkdirSync(dest, { recursive: true });
  }
  
  const entries = fs.readdirSync(src, { withFileTypes: true });
  
  for (let entry of entries) {
    const srcPath = path.join(src, entry.name);
    const destPath = path.join(dest, entry.name);
    
    if (entry.isDirectory()) {
      copyDir(srcPath, destPath);
    } else {
      fs.copyFileSync(srcPath, destPath);
    }
  }
}

// Main build process
console.log('Starting build process...');

// Clean static directory
const staticDir = path.join(__dirname, '..', 'static');
if (fs.existsSync(staticDir)) {
  console.log('Cleaning static directory...');
  removeDir(staticDir);
}

// Create static directory
fs.mkdirSync(staticDir, { recursive: true });

// Run react-scripts build with GENERATE_SOURCEMAP
console.log('Building React app...');
try {
  // Set environment variable in a cross-platform way
  const env = { ...process.env, GENERATE_SOURCEMAP: 'true' };
  execSync('npx react-scripts build', { 
    stdio: 'inherit',
    env: env,
    cwd: path.join(__dirname, '..')
  });
} catch (error) {
  console.error('Build failed:', error.message);
  process.exit(1);
}

// Copy build files to static
const buildDir = path.join(__dirname, '..', 'build');
console.log('Copying build files to static...');

// Copy static assets
const buildStaticDir = path.join(buildDir, 'static');
if (fs.existsSync(buildStaticDir)) {
  copyDir(buildStaticDir, staticDir);
}

// Copy individual files
const filesToCopy = ['index.html', 'asset-manifest.json'];
for (const file of filesToCopy) {
  const srcFile = path.join(buildDir, file);
  const destFile = path.join(staticDir, file);
  if (fs.existsSync(srcFile)) {
    fs.copyFileSync(srcFile, destFile);
  }
}

// Copy screenshots if they exist
const screenshotsDir = path.join(buildDir, 'screenshots');
if (fs.existsSync(screenshotsDir)) {
  const destScreenshotsDir = path.join(staticDir, 'screenshots');
  copyDir(screenshotsDir, destScreenshotsDir);
}

console.log('Build completed successfully!'); 