import { describe, it, expect } from 'vitest';
import * as fc from 'fast-check';
import type { ProtoMessageSchema, ProtoField } from '$lib/types';

/**
 * Property-based tests for schema-to-completion-items mapping
 * and schema-to-validation logic.
 *
 * Since the CodeMirror extension internals are not directly importable as pure functions,
 * we test the underlying logic by reimplementing the core algorithms that
 * schema-autocomplete.ts and schema-lint.ts use. These mirror the actual implementation
 * and test that the schema → completions/diagnostics mapping is correct for any schema.
 */

// --- Reimplemented pure functions matching the actual implementation logic ---

/** Mirrors getFieldNameCompletions logic: maps schema fields to completion items */
function getFieldNameCompletions(fields: ProtoField[]): Array<{ label: string; type: string; detail: string }> {
	return fields.slice(0, 20).map((f) => ({
		label: f.json_name,
		type: f.message_fields ? 'property' : 'variable',
		detail: formatFieldDetail(f),
	}));
}

/** Mirrors formatFieldDetail from schema-autocomplete.ts */
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

// --- Validation logic mirroring schema-lint.ts ---

const NUMERIC_TYPES = new Set([
	'int32', 'int64', 'uint32', 'uint64', 'sint32', 'sint64',
	'fixed32', 'fixed64', 'sfixed32', 'sfixed64', 'float', 'double',
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

interface ValidationError {
	field: string;
	message: string;
}

/** Validates a JSON object against a proto schema, returning errors */
function validateObject(
	obj: Record<string, unknown>,
	fields: ProtoField[],
): ValidationError[] {
	const errors: ValidationError[] = [];
	const fieldMap = new Map<string, ProtoField>();
	for (const field of fields) {
		fieldMap.set(field.json_name, field);
	}

	for (const key of Object.keys(obj)) {
		const field = fieldMap.get(key);
		if (!field) {
			errors.push({ field: key, message: `Unknown field "${key}"` });
			continue;
		}

		const value = obj[key];
		const expected = expectedJsonType(field);
		const actual = actualJsonType(value);

		if (expected && actual !== 'null' && actual !== expected) {
			errors.push({
				field: key,
				message: `Type mismatch: field "${key}" expects ${expected}, got ${actual}`,
			});
		}
	}

	return errors;
}

function actualJsonType(value: unknown): string {
	if (value === null) return 'null';
	if (Array.isArray(value)) return 'array';
	return typeof value;
}

// --- fast-check Arbitraries ---

const PROTO_SCALAR_TYPES = [
	'string', 'int32', 'int64', 'uint32', 'uint64',
	'float', 'double', 'bool', 'bytes',
] as const;

const PROTO_ALL_TYPES = [
	...PROTO_SCALAR_TYPES, 'enum', 'message',
] as const;

/** Generates a valid field name (valid JSON key, non-empty, no special chars) */
const fieldNameArb = fc.stringMatching(/^[a-z][a-z0-9_]{0,19}$/).filter((s) => s.length > 0);

/** Generates a ProtoField with random name and type */
const protoFieldArb: fc.Arbitrary<ProtoField> = fc.record({
	name: fieldNameArb,
	json_name: fieldNameArb,
	type: fc.constantFrom(...PROTO_ALL_TYPES),
	repeated: fc.boolean(),
	map_key_type: fc.option(fc.constantFrom('string', 'int32', 'int64'), { nil: undefined }),
	map_value_type: fc.option(fc.constantFrom('string', 'int32', 'message'), { nil: undefined }),
	enum_values: fc.option(fc.array(fc.stringMatching(/^[A-Z][A-Z0-9_]{0,9}$/), { minLength: 1, maxLength: 5 }), { nil: undefined }),
	message_fields: fc.constant(undefined),
	comment: fc.option(fc.string({ minLength: 0, maxLength: 50 }), { nil: undefined }),
}).map((f) => {
	// Ensure json_name and name are consistent (json_name is camelCase of name).
	const field: ProtoField = { ...f, json_name: f.name };
	// Fix map fields: both key and value must be set together.
	if (field.map_key_type && !field.map_value_type) {
		field.map_value_type = 'string';
	}
	if (field.map_value_type && !field.map_key_type) {
		field.map_key_type = 'string';
	}
	// If type is enum, ensure enum_values is populated.
	if (field.type === 'enum' && !field.enum_values) {
		field.enum_values = ['UNKNOWN', 'VALUE_A', 'VALUE_B'];
	}
	return field;
});

/** Generates a ProtoMessageSchema with unique field names */
const protoSchemaArb: fc.Arbitrary<ProtoMessageSchema> = fc
	.array(protoFieldArb, { minLength: 1, maxLength: 15 })
	.map((fields) => {
		// Deduplicate field names.
		const seen = new Set<string>();
		const uniqueFields = fields.filter((f) => {
			if (seen.has(f.json_name)) return false;
			seen.add(f.json_name);
			return true;
		});
		return {
			full_name: 'test.TestMessage',
			fields: uniqueFields.length > 0 ? uniqueFields : [{ name: 'id', json_name: 'id', type: 'string', repeated: false }],
		};
	});

// --- Tests ---

describe('Schema-to-completion-items mapping (Property-based)', () => {
	it('every field in schema produces a completion item', () => {
		fc.assert(
			fc.property(protoSchemaArb, (schema) => {
				const completions = getFieldNameCompletions(schema.fields);

				// Property: number of completion items equals number of fields (up to 20 max).
				const expectedCount = Math.min(schema.fields.length, 20);
				expect(completions).toHaveLength(expectedCount);

				// Property: every completion label matches a field's json_name.
				const fieldNames = new Set(schema.fields.slice(0, 20).map((f) => f.json_name));
				for (const completion of completions) {
					expect(fieldNames.has(completion.label)).toBe(true);
				}
			}),
			{ numRuns: 100 },
		);
	});

	it('completion items have correct type based on field structure', () => {
		fc.assert(
			fc.property(protoSchemaArb, (schema) => {
				const completions = getFieldNameCompletions(schema.fields);

				for (let i = 0; i < completions.length; i++) {
					const field = schema.fields[i];
					const completion = completions[i];

					// Property: message fields get "property" type, others get "variable".
					if (field.message_fields) {
						expect(completion.type).toBe('property');
					} else {
						expect(completion.type).toBe('variable');
					}
				}
			}),
			{ numRuns: 100 },
		);
	});

	it('completion detail reflects field type correctly', () => {
		fc.assert(
			fc.property(protoSchemaArb, (schema) => {
				const completions = getFieldNameCompletions(schema.fields);

				for (let i = 0; i < completions.length; i++) {
					const field = schema.fields[i];
					const completion = completions[i];

					// Property: map fields show map<K, V> detail.
					if (field.map_key_type && field.map_value_type) {
						expect(completion.detail).toBe(`map<${field.map_key_type}, ${field.map_value_type}>`);
					}
					// Property: repeated non-map fields show "repeated type" detail.
					else if (field.repeated) {
						expect(completion.detail).toBe(`repeated ${field.type}`);
					}
					// Property: plain fields show just the type.
					else {
						expect(completion.detail).toBe(field.type);
					}
				}
			}),
			{ numRuns: 100 },
		);
	});
});

describe('Schema-to-validation logic (Property-based)', () => {
	/** Generate a conforming value for a given field type */
	function conformingValueArb(field: ProtoField): fc.Arbitrary<unknown> {
		if (field.repeated) return fc.constant([]);
		if (field.map_key_type) return fc.constant({});
		if (NUMERIC_TYPES.has(field.type)) return fc.double({ min: -1e6, max: 1e6, noNaN: true });
		if (field.type === 'bool') return fc.boolean();
		if (field.type === 'string' || field.type === 'bytes') return fc.string({ minLength: 0, maxLength: 20 });
		if (field.type === 'message') return fc.constant({});
		if (field.type === 'enum') {
			if (field.enum_values && field.enum_values.length > 0) {
				return fc.constantFrom(...field.enum_values);
			}
			return fc.string({ minLength: 1, maxLength: 10 });
		}
		return fc.constant(null);
	}

	it('conforming JSON object produces no validation errors', () => {
		fc.assert(
			fc.property(protoSchemaArb, fc.context(), (schema, ctx) => {
				// Build a conforming object with all fields set to correct types.
				const obj: Record<string, unknown> = {};
				for (const field of schema.fields) {
					const value = fc.sample(conformingValueArb(field), 1)[0];
					obj[field.json_name] = value;
					ctx.log(`${field.json_name}: ${JSON.stringify(value)} (type: ${field.type}, repeated: ${field.repeated})`);
				}

				const errors = validateObject(obj, schema.fields);
				expect(errors).toHaveLength(0);
			}),
			{ numRuns: 100 },
		);
	});

	it('unknown fields are detected as errors', () => {
		fc.assert(
			fc.property(
				protoSchemaArb,
				fc.stringMatching(/^unknown_[a-z]{1,10}$/),
				(schema, unknownFieldName) => {
					// Ensure the unknown field name doesn't collide with actual field names.
					const fieldNames = new Set(schema.fields.map((f) => f.json_name));
					fc.pre(!fieldNames.has(unknownFieldName));

					// Build an object with one unknown field.
					const obj: Record<string, unknown> = {
						[unknownFieldName]: 'some_value',
					};

					const errors = validateObject(obj, schema.fields);

					// Property: at least one error mentioning the unknown field.
					expect(errors.length).toBeGreaterThanOrEqual(1);
					const unknownError = errors.find((e) => e.field === unknownFieldName);
					expect(unknownError).toBeDefined();
					expect(unknownError!.message).toContain('Unknown field');
				},
			),
			{ numRuns: 100 },
		);
	});

	it('type mismatches are detected', () => {
		// For each numeric field, passing a string should produce a type mismatch.
		const numericFieldArb = fc.record({
			name: fieldNameArb,
			json_name: fieldNameArb,
			type: fc.constantFrom('int32', 'int64', 'float', 'double'),
			repeated: fc.constant(false),
		}).map((f) => ({ ...f, json_name: f.name }) as ProtoField);

		fc.assert(
			fc.property(numericFieldArb, fc.string({ minLength: 1, maxLength: 10 }), (field, stringValue) => {
				const schema: ProtoMessageSchema = {
					full_name: 'test.Msg',
					fields: [field],
				};

				// Put a string where a number is expected.
				const obj: Record<string, unknown> = {
					[field.json_name]: stringValue,
				};

				const errors = validateObject(obj, schema.fields);

				// Property: there should be a type mismatch error.
				expect(errors.length).toBeGreaterThanOrEqual(1);
				const mismatchError = errors.find((e) => e.field === field.json_name);
				expect(mismatchError).toBeDefined();
				expect(mismatchError!.message).toContain('Type mismatch');
			}),
			{ numRuns: 100 },
		);
	});

	it('boolean fields reject string values', () => {
		const boolFieldArb = fc.record({
			name: fieldNameArb,
			json_name: fieldNameArb,
			type: fc.constant('bool' as const),
			repeated: fc.constant(false),
		}).map((f) => ({ ...f, json_name: f.name }) as ProtoField);

		fc.assert(
			fc.property(boolFieldArb, fc.string({ minLength: 1, maxLength: 10 }), (field, stringValue) => {
				const schema: ProtoMessageSchema = {
					full_name: 'test.Msg',
					fields: [field],
				};

				const obj: Record<string, unknown> = {
					[field.json_name]: stringValue,
				};

				const errors = validateObject(obj, schema.fields);
				expect(errors.length).toBeGreaterThanOrEqual(1);
				expect(errors[0].message).toContain('Type mismatch');
			}),
			{ numRuns: 100 },
		);
	});
});
