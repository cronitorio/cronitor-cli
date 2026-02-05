const puppeteer = require('puppeteer');
const fs = require('fs');
const path = require('path');

const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

async function captureScreenshots() {
  // Create screenshots directory if it doesn't exist
  const screenshotsDir = path.join(__dirname, '../public/screenshots');
  if (!fs.existsSync(screenshotsDir)) {
    fs.mkdirSync(screenshotsDir, { recursive: true });
  }

  const browser = await puppeteer.launch();
  const page = await browser.newPage();
  await page.setViewport({ width: 1440, height: 900 });
  
  // Set basic authentication
  await page.authenticate({
    username: 'admin',
    password: 'admin123'
  });
  
  try {
    console.log('🚀 Starting screenshot capture...');
    
    // Navigate to the app and wait for it to load
    await page.goto('http://localhost:9000', { waitUntil: 'networkidle2' });
    await delay(3000);
    
    // Capture main pages
    const pages = [
      { name: 'jobs-dark', path: '/', filename: 'jobs-dark.png' },
      { name: 'crontabs', path: '/crontabs', filename: 'crontabs-dark.png' },
      { name: 'settings', path: '/settings', filename: 'settings-dark.png' },
      { name: 'docs', path: '/docs', filename: 'docs-dark.png' }
    ];
    
    for (const pageInfo of pages) {
      try {
        if (pageInfo.path !== '/') {
          await page.goto(`http://localhost:9000${pageInfo.path}`, { waitUntil: 'networkidle2' });
          await delay(2000);
        }
        
        await page.screenshot({ 
          path: path.join(screenshotsDir, pageInfo.filename), 
          fullPage: false 
        });
        console.log(`✓ ${pageInfo.name} page captured`);
      } catch (e) {
        console.log(`⚠️  Could not capture ${pageInfo.name} page: ${e.message}`);
      }
    }
    
    // Capture sidebar
    await page.goto('http://localhost:9000', { waitUntil: 'networkidle2' });
    await delay(2000);
    await page.screenshot({ 
      path: path.join(screenshotsDir, 'sidebar-dark.png'), 
      clip: { x: 0, y: 0, width: 256, height: 900 }
    });
    console.log('✓ Sidebar captured');
    
    // Try to capture light mode
    try {
      await page.evaluate(() => {
        const buttons = Array.from(document.querySelectorAll('button'));
        const toggleButton = buttons.find(btn => 
          btn.getAttribute('role') === 'switch' ||
          (btn.querySelector('svg') && btn.closest('div')?.textContent?.includes('Mode'))
        );
        if (toggleButton) toggleButton.click();
        return !!toggleButton;
      });
      
      await delay(1500);
      await page.screenshot({ 
        path: path.join(screenshotsDir, 'jobs-light.png'), 
        fullPage: false 
      });
      console.log('✓ Jobs page (light mode) captured');
    } catch (e) {
      console.log('⚠️  Could not capture light mode');
    }
    
  } catch (error) {
    console.error('❌ Error during screenshot capture:', error);
  }
  
  await browser.close();
  console.log('📸 Screenshot capture complete!');
  
  // List captured files
  console.log('\n📁 Captured screenshots:');
  const files = fs.readdirSync(screenshotsDir);
  files.forEach(file => {
    const filePath = path.join(screenshotsDir, file);
    const stats = fs.statSync(filePath);
    console.log(`   ${file} (${(stats.size / 1024).toFixed(1)}KB)`);
  });
}

// Run if called directly
if (require.main === module) {
  captureScreenshots().catch(console.error);
}

module.exports = { captureScreenshots }; 