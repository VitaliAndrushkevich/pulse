/**
 * Generate PNG brand assets from the logo-mark SVG.
 *
 * Usage: node scripts/generate-brand-pngs.mjs
 *
 * Produces:
 *   static/brand/logo-mark-1x.png  (64×64)
 *   static/brand/logo-mark-2x.png  (128×128)
 *   static/brand/logo-mark-4x.png  (256×256)
 */
import sharp from 'sharp';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const brandDir = resolve(__dirname, '..', 'static', 'brand');
const svgPath = resolve(brandDir, 'logo-mark.svg');

const svgContent = readFileSync(svgPath, 'utf-8');

const sizes = [
  { name: 'logo-mark-1x.png', width: 64, height: 64 },
  { name: 'logo-mark-2x.png', width: 128, height: 128 },
  { name: 'logo-mark-4x.png', width: 256, height: 256 }
];

for (const { name, width, height } of sizes) {
  const resizedSvg = svgContent
    .replace(/width="\d+"/, `width="${width}"`)
    .replace(/height="\d+"/, `height="${height}"`);

  await sharp(Buffer.from(resizedSvg))
    .resize(width, height)
    .png()
    .toFile(resolve(brandDir, name));

  console.log(`Generated ${name} (${width}×${height})`);
}

console.log('Done.');
