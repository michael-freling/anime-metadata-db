// Capture screenshots of the docs site with headless Chromium.
// Usage: node shoot.mjs <baseURL> <outDir> <path> [<path> ...]
// For each path: a full-page desktop (light) shot. For the FIRST path (the
// landing page): also a dark-mode and a mobile shot.
// Must run from a dir whose node_modules contains playwright (the caller copies
// this file next to that install).
import { chromium } from 'playwright';

const [base, outDir, ...paths] = process.argv.slice(2);
if (!base || !outDir || paths.length === 0) {
  console.error('usage: node shoot.mjs <baseURL> <outDir> <path> [<path> ...]');
  process.exit(1);
}

const label = (p) => (p === '/' ? 'home' : p.replace(/^\/+|\/+$/g, '').replace(/\//g, '-') || 'home');
const join = (p) => base.replace(/\/$/, '') + p;

const browser = await chromium.launch();
try {
  for (let i = 0; i < paths.length; i++) {
    const p = paths[i];
    const l = label(p);
    const url = join(p);

    const page = await browser.newPage({ viewport: { width: 1280, height: 860 }, deviceScaleFactor: 2 });
    await page.goto(url, { waitUntil: 'networkidle', timeout: 30000 });
    await page.waitForTimeout(400);
    await page.screenshot({ path: `${outDir}/${l}-light.png`, fullPage: true });
    console.log(`${outDir}/${l}-light.png`);

    if (i === 0) {
      const dark = await browser.newPage({ viewport: { width: 1280, height: 860 }, deviceScaleFactor: 2, colorScheme: 'dark' });
      await dark.goto(url, { waitUntil: 'networkidle', timeout: 30000 });
      await dark.waitForTimeout(400);
      await dark.screenshot({ path: `${outDir}/${l}-dark.png` });
      console.log(`${outDir}/${l}-dark.png`);

      const mob = await browser.newPage({ viewport: { width: 390, height: 800 }, deviceScaleFactor: 2 });
      await mob.goto(url, { waitUntil: 'networkidle', timeout: 30000 });
      await mob.waitForTimeout(400);
      await mob.screenshot({ path: `${outDir}/${l}-mobile.png`, fullPage: true });
      console.log(`${outDir}/${l}-mobile.png`);
    }
  }
} finally {
  await browser.close();
}
