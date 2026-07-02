/**
 * Build-time validation script for locale translation files.
 *
 * Checks:
 * 1. Each supported locale has a corresponding .json file in src/locales/
 * 2. All keys in en.json exist in other locale files (warns on missing)
 * 3. No key path exceeds 4 levels of nesting
 *
 * Exit codes:
 *   0 — all checks passed (warnings are non-fatal)
 *   1 — critical failure (missing files or structural violations)
 */

import fs from 'node:fs';
import path from 'node:path';

// --- Configuration (mirrors src/lib/i18n/config.ts) ---

const SUPPORTED_LOCALES = [
  { code: 'en', name: 'English' },
  { code: 'ru', name: 'Русский' },
  { code: 'es', name: 'Español' },
  { code: 'fr', name: 'Français' },
  { code: 'pt', name: 'Português' },
  { code: 'de', name: 'Deutsch' },
  { code: 'zh', name: '中文' },
  { code: 'ja', name: '日本語' },
  { code: 'ko', name: '한국어' },
  { code: 'tr', name: 'Türkçe' },
  { code: 'it', name: 'Italiano' },
] as const;

const LOCALES_DIR = path.resolve(import.meta.dirname, '../src/locales');
const MAX_NESTING_DEPTH = 4;

// --- Helpers ---

interface ValidationResult {
  errors: string[];
  warnings: string[];
}

/**
 * Extract all leaf key paths from a nested object using dot notation.
 * Tracks the current depth to detect nesting violations.
 *
 * A key path like "a.b.c.d" has depth 4 (4 segments). A key like "a.b.c.d.e"
 * has depth 5 and violates the MAX_NESTING_DEPTH=4 rule.
 */
function extractKeys(
  obj: Record<string, unknown>,
  prefix: string = '',
  depth: number = 1,
  nestingViolations: string[] = []
): { keys: string[]; nestingViolations: string[] } {
  const keys: string[] = [];

  for (const key of Object.keys(obj)) {
    const fullPath = prefix ? `${prefix}.${key}` : key;
    const value = obj[key];

    if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
      const nested = extractKeys(
        value as Record<string, unknown>,
        fullPath,
        depth + 1,
        nestingViolations
      );
      keys.push(...nested.keys);
    } else {
      // Leaf value — check if the path depth exceeds the limit
      if (depth > MAX_NESTING_DEPTH) {
        nestingViolations.push(fullPath);
      }
      keys.push(fullPath);
    }
  }

  return { keys, nestingViolations };
}

/**
 * Validate that all locale files exist and have valid structure.
 */
function validate(): ValidationResult {
  const errors: string[] = [];
  const warnings: string[] = [];

  // Step 1: Check that each supported locale has a .json file
  console.log('\n📁 Checking locale file existence...\n');

  const missingFiles: string[] = [];

  for (const locale of SUPPORTED_LOCALES) {
    const filePath = path.join(LOCALES_DIR, `${locale.code}.json`);
    if (fs.existsSync(filePath)) {
      console.log(`  ✓ ${locale.code}.json exists`);
    } else {
      console.log(`  ✗ ${locale.code}.json is MISSING`);
      missingFiles.push(locale.code);
      errors.push(`Missing locale file: ${locale.code}.json`);
    }
  }

  if (missingFiles.length > 0) {
    // Critical failure — cannot proceed with key validation
    return { errors, warnings };
  }

  // Step 2: Load en.json and extract all leaf keys
  console.log('\n🔑 Extracting keys from en.json...\n');

  const enPath = path.join(LOCALES_DIR, 'en.json');
  const enContent = JSON.parse(fs.readFileSync(enPath, 'utf-8'));
  const { keys: enKeys, nestingViolations: enNestingViolations } = extractKeys(enContent);

  console.log(`  ✓ Found ${enKeys.length} translation keys in en.json`);

  // Step 3: Check nesting depth in all locale files
  console.log(`\n📐 Checking nesting depth (max ${MAX_NESTING_DEPTH} levels)...\n`);

  // Check en.json nesting violations
  if (enNestingViolations.length > 0) {
    for (const key of enNestingViolations) {
      console.log(`  ✗ en.json: key "${key}" exceeds ${MAX_NESTING_DEPTH} levels`);
      errors.push(`en.json: key "${key}" exceeds ${MAX_NESTING_DEPTH} levels of nesting`);
    }
  } else {
    console.log(`  ✓ en.json — all keys within ${MAX_NESTING_DEPTH} levels`);
  }

  // Check other locale files for nesting violations
  for (const locale of SUPPORTED_LOCALES) {
    if (locale.code === 'en') continue;

    const filePath = path.join(LOCALES_DIR, `${locale.code}.json`);
    let content: Record<string, unknown>;

    try {
      content = JSON.parse(fs.readFileSync(filePath, 'utf-8'));
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      console.log(`  ✗ ${locale.code}.json — invalid JSON: ${msg}`);
      errors.push(`${locale.code}.json: invalid JSON — ${msg}`);
      continue;
    }

    const { nestingViolations } = extractKeys(content);

    if (nestingViolations.length > 0) {
      for (const key of nestingViolations) {
        console.log(`  ✗ ${locale.code}.json: key "${key}" exceeds ${MAX_NESTING_DEPTH} levels`);
        errors.push(
          `${locale.code}.json: key "${key}" exceeds ${MAX_NESTING_DEPTH} levels of nesting`
        );
      }
    } else {
      console.log(`  ✓ ${locale.code}.json — all keys within ${MAX_NESTING_DEPTH} levels`);
    }
  }

  // Step 4: Check that all en.json keys exist in other locale files
  console.log('\n🌐 Checking key coverage in other locales...\n');

  for (const locale of SUPPORTED_LOCALES) {
    if (locale.code === 'en') continue;

    const filePath = path.join(LOCALES_DIR, `${locale.code}.json`);
    let content: Record<string, unknown>;

    try {
      content = JSON.parse(fs.readFileSync(filePath, 'utf-8'));
    } catch {
      // Already reported as error above
      continue;
    }

    const { keys: localeKeys } = extractKeys(content);
    const localeKeySet = new Set(localeKeys);
    const missingKeys = enKeys.filter((key) => !localeKeySet.has(key));

    if (missingKeys.length === 0) {
      console.log(`  ✓ ${locale.code}.json — all ${enKeys.length} keys present`);
    } else {
      console.log(
        `  ⚠ ${locale.code}.json — missing ${missingKeys.length} of ${enKeys.length} keys:`
      );
      // Show up to 10 missing keys for readability
      const shown = missingKeys.slice(0, 10);
      for (const key of shown) {
        console.log(`      - ${key}`);
      }
      if (missingKeys.length > 10) {
        console.log(`      ... and ${missingKeys.length - 10} more`);
      }
      warnings.push(
        `${locale.code}.json: missing ${missingKeys.length} keys from en.json`
      );
    }
  }

  return { errors, warnings };
}

// --- Main ---

console.log('╔══════════════════════════════════════╗');
console.log('║   Pulse Locale Validation Script     ║');
console.log('╚══════════════════════════════════════╝');

const { errors, warnings } = validate();

console.log('\n' + '─'.repeat(40));
console.log('\n📊 Summary:\n');

if (warnings.length > 0) {
  console.log(`  ⚠ ${warnings.length} warning(s)`);
  for (const w of warnings) {
    console.log(`    - ${w}`);
  }
}

if (errors.length > 0) {
  console.log(`  ✗ ${errors.length} error(s)`);
  for (const e of errors) {
    console.log(`    - ${e}`);
  }
  console.log('\n❌ Validation FAILED — critical errors found.\n');
  process.exit(1);
} else if (warnings.length > 0) {
  console.log('\n✅ Validation PASSED with warnings (non-critical).\n');
  process.exit(0);
} else {
  console.log('  ✓ All checks passed — no issues found.');
  console.log('\n✅ Validation PASSED.\n');
  process.exit(0);
}
