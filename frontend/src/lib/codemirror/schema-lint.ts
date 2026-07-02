import { linter, type Diagnostic } from '@codemirror/lint';
import type { ProtoMessageSchema, ProtoField } from '$lib/types';

/** Proto type categories mapped to JSON value types */
const NUMERIC_TYPES = new Set([
	'int32',
	'int64',
	'uint32',
	'uint64',
	'sint32',
	'sint64',
	'fixed32',
	'fixed64',
	'sfixed32',
	'sfixed64',
	'float',
	'double'
]);

function expectedJsonType(field: ProtoField): string | null {
	if (field.repeated) return 'array';
	if (field.map_key_type) return 'object';
	if (NUMERIC_TYPES.has(field.type)) return 'number';
	if (field.type === 'bool') return 'boolean';
	if (field.type === 'string' || field.type === 'bytes') return 'string';
	if (field.type === 'message') return 'object';
	if (field.type === 'enum') return 'string';
	return null;
}

function actualJsonType(value: unknown): string {
	if (value === null) return 'null';
	if (Array.isArray(value)) return 'array';
	return typeof value;
}

/**
 * Find the character position of a JSON key in the document text.
 * Uses regex to locate `"key":` patterns and returns the start of the key string.
 */
function findKeyPosition(doc: string, key: string, startFrom: number = 0): number {
	const escaped = key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	const regex = new RegExp(`"${escaped}"\\s*:`, 'g');
	regex.lastIndex = startFrom;
	const match = regex.exec(doc);
	return match ? match.index : -1;
}

/**
 * Validate a parsed object against the proto schema and collect diagnostics.
 */
function validateObject(
	obj: Record<string, unknown>,
	fields: ProtoField[],
	doc: string,
	diagnostics: Diagnostic[],
	searchFrom: number = 0
): void {
	const fieldMap = new Map<string, ProtoField>();
	for (const field of fields) {
		fieldMap.set(field.json_name, field);
	}

	for (const key of Object.keys(obj)) {
		const pos = findKeyPosition(doc, key, searchFrom);
		if (pos === -1) continue;

		const field = fieldMap.get(key);

		if (!field) {
			// Unknown field
			diagnostics.push({
				from: pos,
				to: pos + key.length + 2, // include quotes
				severity: 'warning',
				message: `Unknown field "${key}"`
			});
			continue;
		}

		// Type mismatch check
		const value = obj[key];
		const expected = expectedJsonType(field);
		const actual = actualJsonType(value);

		if (expected && actual !== 'null' && actual !== expected) {
			// Position at value: after `"key": `
			const valueStart = pos + key.length + 2; // rough estimate past the colon
			diagnostics.push({
				from: pos,
				to: pos + key.length + 2,
				severity: 'error',
				message: `Type mismatch: field "${key}" expects ${expected}, got ${actual}`
			});
		}

		// Recurse into nested message fields
		if (
			field.type === 'message' &&
			field.message_fields &&
			actual === 'object' &&
			value !== null &&
			!Array.isArray(value)
		) {
			validateObject(
				value as Record<string, unknown>,
				field.message_fields,
				doc,
				diagnostics,
				pos
			);
		}

		// Recurse into repeated message items
		if (
			field.repeated &&
			field.type === 'message' &&
			field.message_fields &&
			Array.isArray(value)
		) {
			for (const item of value) {
				if (item !== null && typeof item === 'object' && !Array.isArray(item)) {
					validateObject(
						item as Record<string, unknown>,
						field.message_fields,
						doc,
						diagnostics,
						pos
					);
				}
			}
		}
	}
}

/**
 * CodeMirror 6 linter extension that validates JSON content against a protobuf message schema.
 * Reports unknown fields and type mismatches as inline diagnostics.
 */
export function schemaLint(schema: ProtoMessageSchema) {
	return linter(
		(view) => {
			const diagnostics: Diagnostic[] = [];
			const doc = view.state.doc.toString();

			if (!doc.trim()) {
				return diagnostics;
			}

			let parsed: unknown;
			try {
				parsed = JSON.parse(doc);
			} catch {
				diagnostics.push({
					from: 0,
					to: Math.min(doc.length, 1),
					severity: 'error',
					message: 'Invalid JSON'
				});
				return diagnostics;
			}

			if (parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)) {
				validateObject(parsed as Record<string, unknown>, schema.fields, doc, diagnostics);
			}

			return diagnostics;
		},
		{ delay: 500 }
	);
}
