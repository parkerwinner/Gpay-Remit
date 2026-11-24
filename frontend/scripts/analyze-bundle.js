const fs = require('fs');
const path = require('path');
const zlib = require('zlib');

const assetsDir = path.join(__dirname, '..', 'build', 'static', 'js');
if (!fs.existsSync(assetsDir)) {
  console.error('Frontend build not found. Run npm run build first.');
  process.exit(1);
}

const assets = fs.readdirSync(assetsDir)
  .filter((name) => name.endsWith('.js'))
  .map((name) => {
    const contents = fs.readFileSync(path.join(assetsDir, name));
    return { name, bytes: contents.length, gzipBytes: zlib.gzipSync(contents).length };
  })
  .sort((a, b) => a.name.localeCompare(b.name));

const totalBytes = assets.reduce((sum, asset) => sum + asset.bytes, 0);
const mainAsset = assets.find((asset) => asset.name.startsWith('main.'));
const mainMapPath = mainAsset && path.join(assetsDir, mainAsset.name.replace(/\.js$/, '.js.map'));
const moduleSizes = [];
if (mainMapPath && fs.existsSync(mainMapPath)) {
  const sourceMap = JSON.parse(fs.readFileSync(mainMapPath, 'utf8'));
  const packages = new Map();
  for (let i = 0; i < sourceMap.sources.length; i += 1) {
    const source = sourceMap.sources[i];
    const match = source.match(/node_modules\/(?:((?:@[^/]+\/)?[^/]+))/);
    if (!match) continue;
    const packageName = match[1];
    const sourceBytes = Buffer.byteLength(sourceMap.sourcesContent?.[i] || '');
    packages.set(packageName, (packages.get(packageName) || 0) + sourceBytes);
  }
  for (const [name, bytes] of packages) moduleSizes.push({ name, sourceBytes: bytes });
  moduleSizes.sort((a, b) => b.sourceBytes - a.sourceBytes);
}
const report = {
  generatedAt: new Date().toISOString(),
  assets,
  totalBytes,
  totalGzipBytes: assets.reduce((sum, asset) => sum + asset.gzipBytes, 0),
  largestPackages: moduleSizes.slice(0, 25),
};
const output = path.join(__dirname, '..', 'build', 'bundle-report.json');
fs.writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`);

console.log('JavaScript bundle sizes:');
for (const asset of assets) {
  console.log(`  ${asset.name}: ${(asset.bytes / 1024).toFixed(1)} KiB raw, ${(asset.gzipBytes / 1024).toFixed(1)} KiB gzip`);
}
console.log(`  Total: ${(totalBytes / 1024).toFixed(1)} KiB raw, ${(report.totalGzipBytes / 1024).toFixed(1)} KiB gzip`);
if (moduleSizes.length) {
  console.log('Largest package source contributions in the main bundle:');
  for (const item of moduleSizes.slice(0, 10)) console.log(`  ${item.name}: ${(item.sourceBytes / 1024).toFixed(1)} KiB source`);
}
console.log(`Report: ${path.relative(process.cwd(), output)}`);

const totalBudget = Number(process.env.BUNDLE_TOTAL_BUDGET_KB || 6144) * 1024;
const entryBudget = Number(process.env.BUNDLE_ENTRY_BUDGET_KB || 3072) * 1024;
const entry = assets.find((asset) => asset.name.startsWith('main.'));
if (!entry) {
  console.error('Unable to find the main JavaScript bundle.');
  process.exit(1);
}
if (entry.bytes > entryBudget) {
  console.error(`Main bundle exceeds ${entryBudget / 1024} KiB budget.`);
  process.exitCode = 1;
}
if (totalBytes > totalBudget) {
  console.error(`Total JavaScript exceeds ${totalBudget / 1024} KiB budget.`);
  process.exitCode = 1;
}
