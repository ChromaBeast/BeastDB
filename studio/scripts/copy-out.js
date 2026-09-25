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
  const oldAssets = path.resolve(destDir, '_next');
  if (path.dirname(oldAssets) !== path.resolve(destDir)) {
    throw new Error('Refusing to remove assets outside the static directory');
  }
  fs.rmSync(oldAssets, { recursive: true, force: true });
  copyRecursiveSync(srcDir, destDir);
  console.log('Successfully updated internal/web/static with Next.js export bundle.');
} else {
  console.error('Error: out/ directory does not exist. Run "next build" first.');
  process.exit(1);
}
