const fs = require('fs');
const path = require('path');

const srcDir = path.join(__dirname, '..', 'out');
const destDir = path.join(__dirname, '..', '..', 'internal', 'web', 'static');

function copyRecursiveSync(src, dest) {
  const exists = fs.existsSync(src);
  const stats = exists && fs.statSync(src);
  const isDirectory = exists && stats.isDirectory();
  if (isDirectory) {
    if (!fs.existsSync(dest)) {
      fs.mkdirSync(dest, { recursive: true });
    }
    fs.readdirSync(src).forEach((childItemName) => {
      copyRecursiveSync(path.join(src, childItemName), path.join(dest, childItemName));
    });
  } else {
    fs.copyFileSync(src, dest);
  }
}

if (fs.existsSync(srcDir)) {
  console.log(`Copying static export from ${srcDir} to ${destDir}...`);
  copyRecursiveSync(srcDir, destDir);
  console.log('Successfully updated internal/web/static with Next.js export bundle.');
} else {
  console.error('Error: out/ directory does not exist. Run "next build" first.');
  process.exit(1);
}
