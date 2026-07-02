import { autocompletion, type CompletionContext, type CompletionResult } from '@codemirror/autocomplete';
import type { Extension } from '@codemirror/state';
import type { ProtoMessageSchema, ProtoField } from '$lib/types';

/**
 * Creates a CodeMirror 6 autocompletion extension that provides
 * field name and enum value suggestions based on a ProtoMessageSchema.
 */
export function schemaAutocomplete(schema: ProtoMessageSchema): Extension {
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  return autocompletion({
    override: [
      (context: CompletionContext): CompletionResult | null | Promise<CompletionResult | null> => {
        // Debounce: return a promise that resolves after 200ms
        if (debounceTimer) {
          clearTimeout(debounceTimer);
        }

        return new Promise((resolve) => {
          debounceTimer = setTimeout(() => {
            resolve(getCompletions(context, schema));
          }, 200);
        });
      },
    ],
    activateOnTyping: true,
    maxRenderedOptions: 20,
  });
}

/**
 * Determines the current cursor context and returns appropriate completions.
 */
function getCompletions(
  context: CompletionContext,
  schema: ProtoMessageSchema,
): CompletionResult | null {
  const textBefore = context.state.doc.sliceString(0, context.pos);

  // Determine the nesting path to figure out which fields are available
  const fields = resolveFieldsAtPath(textBefore, schema.fields);
  if (!fields) return null;

  // Determine if we're in a key position or a value position
  const cursorContext = determineCursorContext(textBefore);

  if (cursorContext.type === 'key') {
    return getFieldNameCompletions(context, fields);
  }

  if (cursorContext.type === 'value' && cursorContext.fieldName) {
    return getValueCompletions(context, fields, cursorContext.fieldName);
  }

  return null;
}

interface CursorContext {
  type: 'key' | 'value' | 'unknown';
  fieldName?: string;
}

/**
 * Uses simple string heuristics to determine if the cursor is at a key
 * position or a value position, and if value, which field key it belongs to.
 */
function determineCursorContext(textBefore: string): CursorContext {
  // Work backwards from the cursor to find the last significant token
  const trimmed = textBefore.trimEnd();

  // Check if we're in a value position: look for `"fieldName":` pattern before cursor
  // Match the last `"key":\s*` that isn't followed by a complete value
  const valueMatch = trimmed.match(/"([^"]+)"\s*:\s*"?[^,}\]]*$/);
  if (valueMatch) {
    // Verify we're actually after the colon (not in a nested key)
    const afterLastColon = trimmed.lastIndexOf(':');
    const afterLastOpenBrace = trimmed.lastIndexOf('{');
    const afterLastComma = trimmed.lastIndexOf(',');

    if (afterLastColon > afterLastOpenBrace && afterLastColon > afterLastComma) {
      return { type: 'value', fieldName: valueMatch[1] };
    }
  }

  // Check if we're in a key position: after `{` or `,` (possibly with whitespace/newlines)
  // Look at what comes before the current word being typed
  const keyContext = trimmed.match(/[{,]\s*"?[^":,{}[\]]*$/);
  if (keyContext) {
    return { type: 'key' };
  }

  // Also handle the case where cursor is right after `{` or `,` with no quote yet
  const lastSignificant = trimmed.slice(-1);
  if (lastSignificant === '{' || lastSignificant === ',') {
    return { type: 'key' };
  }

  return { type: 'unknown' };
}

/**
 * Resolves which ProtoField[] is relevant at the current nesting depth
 * by tracking brace depth in the text before the cursor.
 */
function resolveFieldsAtPath(
  textBefore: string,
  rootFields: ProtoField[],
): ProtoField[] | null {
  const path = getNestedFieldPath(textBefore);

  let currentFields = rootFields;
  for (const fieldName of path) {
    const field = currentFields.find(
      (f) => f.json_name === fieldName || f.name === fieldName,
    );
    if (!field || !field.message_fields) {
      return null;
    }
    currentFields = field.message_fields;
  }

  return currentFields;
}

/**
 * Determines the nested field path by tracking object nesting.
 * For example, if the cursor is inside `{"address": { | }}`,
 * this returns ["address"] so we know to suggest address's subfields.
 */
function getNestedFieldPath(textBefore: string): string[] {
  const path: string[] = [];
  const stack: string[] = [];

  // Simple state machine: track opening braces and the key that preceded them
  let i = 0;
  let inString = false;
  let currentKey = '';
  let lastKey = '';

  while (i < textBefore.length) {
    const ch = textBefore[i];

    if (ch === '\\' && inString) {
      i += 2; // skip escaped character
      continue;
    }

    if (ch === '"') {
      if (!inString) {
        inString = true;
        currentKey = '';
      } else {
        inString = false;
        // Check if this string is a key (followed by ':')
        const rest = textBefore.slice(i + 1).trimStart();
        if (rest.startsWith(':')) {
          lastKey = currentKey;
        }
      }
      i++;
      continue;
    }

    if (inString) {
      currentKey += ch;
      i++;
      continue;
    }

    if (ch === '{') {
      // Opening a new object: if we have a lastKey, it means we're nesting into that field
      stack.push(lastKey);
      if (lastKey) {
        path.push(lastKey);
      }
      lastKey = '';
    } else if (ch === '}') {
      stack.pop();
      if (path.length > 0 && stack.length < path.length) {
        path.pop();
      }
    }

    i++;
  }

  // The first `{` is the root object, so remove the empty entry if present
  // path entries from root-level are empty strings, filter them out
  return path.filter((p) => p.length > 0);
}

/**
 * Returns field name completions for the current nesting level.
 */
function getFieldNameCompletions(
  context: CompletionContext,
  fields: ProtoField[],
): CompletionResult | null {
  // Find word being typed (possibly inside quotes)
  const word = context.matchBefore(/["\w]*/);
  if (!word) return null;

  const from = word.from;
  const typed = word.text.replace(/^"/, '');

  const options = fields
    .filter((f) => f.json_name.toLowerCase().startsWith(typed.toLowerCase()))
    .slice(0, 20)
    .map((f) => ({
      label: f.json_name,
      type: f.message_fields ? 'property' as const : 'variable' as const,
      detail: formatFieldDetail(f),
      info: f.comment || undefined,
      apply: applyFieldName(f, context),
    }));

  if (options.length === 0) return null;

  return {
    from,
    options,
    filter: false,
  };
}

/**
 * Returns enum value completions for a specific field.
 */
function getValueCompletions(
  context: CompletionContext,
  fields: ProtoField[],
  fieldName: string,
): CompletionResult | null {
  const field = fields.find(
    (f) => f.json_name === fieldName || f.name === fieldName,
  );

  if (!field || !field.enum_values || field.enum_values.length === 0) {
    return null;
  }

  // Match the partially typed value (possibly inside quotes)
  const word = context.matchBefore(/["\w]*/);
  if (!word) return null;

  const from = word.from;
  const typed = word.text.replace(/^"/, '');

  const options = field.enum_values
    .filter((v) => v.toLowerCase().startsWith(typed.toLowerCase()))
    .slice(0, 20)
    .map((v) => ({
      label: v,
      type: 'enum' as const,
      detail: `${field.type} enum`,
      apply: `"${v}"`,
    }));

  if (options.length === 0) return null;

  return {
    from,
    options,
    filter: false,
  };
}

/**
 * Formats a human-readable detail string for a field completion item.
 */
function formatFieldDetail(field: ProtoField): string {
  let detail = field.type;
  if (field.repeated) {
    detail = `repeated ${detail}`;
  }
  if (field.map_key_type && field.map_value_type) {
    detail = `map<${field.map_key_type}, ${field.map_value_type}>`;
  }
  return detail;
}

/**
 * Generates the apply text for a field name completion.
 * Wraps in quotes and adds colon.
 */
function applyFieldName(field: ProtoField, context: CompletionContext): string {
  const textBefore = context.state.doc.sliceString(
    Math.max(0, context.pos - 1),
    context.pos,
  );
  const hasQuote = textBefore === '"';

  if (hasQuote) {
    // Already inside a quote — complete the key and add colon
    return `${field.json_name}": `;
  }
  return `"${field.json_name}": `;
}
