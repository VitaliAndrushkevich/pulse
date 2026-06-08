/**
 * Generate favicon and PWA icon PNGs from the ECG logo SVG.
 * Run: node scripts/generate-icons.mjs
 */
import sharp from 'sharp';
import { writeFileSync, mkdirSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const staticDir = resolve(__dirname, '..', 'static');

// ECG logo SVG with brand primary color on transparent background
function logoSvg(size) {
  // Scale the viewBox path to render at the target size
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 32 32" fill="none">
  <path d="M2 20 L8 20 L11 24 L16 8 L21 24 L24 20 L30 20" stroke="#0ea5e9" stroke-width="3.2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;
}

const icons = [
  { name: 'favicon.png', size: 32 },
  { name: 'apple-touch-icon.png', size: 180 },
  { name: 'icon-192.png', size: 192 },
  { name: 'icon-512.png', size: 512 },
];

async function generate() {
  mkdirSync(staticDir, { recursive: true });

  for (const icon of icons) {
    const svg = Buffer.from(logoSvg(icon.size));
    const png = await sharp(svg)
      .resize(icon.size, icon.size)
      .png()
      .toBuffer();

    const outPath = resolve(staticDir, icon.name);
    writeFileSync(outPath, png);
    console.log(`Generated ${icon.name} (${icon.size}×${icon.size})`);
  }
}

generate().catch((err) => {
  console.error('Icon generation failed:', err);
  process.exit(1);
});
