import { hoverTooltip } from '@codemirror/view';
import type { ProtoMessageSchema, ProtoField } from '$lib/types';

const MAX_COMMENT_LENGTH = 256;

/**
 * Build a flat lookup map from json_name to field for quick access.
 */
function buildFieldMap(fields: ProtoField[]): Map<string, ProtoField> {
	const map = new Map<string, ProtoField>();

	function walk(fieldList: ProtoField[]) {
		for (const field of fieldList) {
			map.set(field.json_name, field);
			if (field.message_fields) {
				walk(field.message_fields);
			}
		}
	}

	walk(fields);
	return map;
}

/**
 * Extract the word (unquoted JSON key) at the given position in the document.
 * Returns the word boundaries and text, or null if not on a word.
 */
function getWordAt(
	docText: string,
	pos: number
): { from: number; to: number; word: string } | null {
	// Look for a quoted string containing pos
	// Walk left to find opening quote
	let start = pos;
	while (start > 0 && docText[start - 1] !== '"' && docText[start - 1] !== '\n') {
		start--;
	}

	// Walk right to find closing quote
	let end = pos;
	while (end < docText.length && docText[end] !== '"' && docText[end] !== '\n') {
		end++;
	}

	// Check we're actually inside quotes
	if (start > 0 && docText[start - 1] === '"' && end < docText.length && docText[end] === '"') {
		const word = docText.slice(start, end);
		// Only treat as field key if followed by colon (with optional whitespace)
		const afterQuote = docText.slice(end + 1).trimStart();
		if (afterQuote.startsWith(':')) {
			return { from: start - 1, to: end + 1, word };
		}
	}

	return null;
}

/**
 * CodeMirror 6 hover tooltip extension that shows proto field comments on hover.
 * When the cursor hovers over a JSON field key that matches a schema field with a comment,
 * displays the comment (truncated to 256 characters).
 */
export function schemaTooltip(schema: ProtoMessageSchema) {
	const fieldMap = buildFieldMap(schema.fields);

	return hoverTooltip((view, pos) => {
		const docText = view.state.doc.toString();
		const wordInfo = getWordAt(docText, pos);

		if (!wordInfo) return null;

		const field = fieldMap.get(wordInfo.word);
		if (!field || !field.comment) return null;

		let comment = field.comment;
		if (comment.length > MAX_COMMENT_LENGTH) {
			comment = comment.slice(0, MAX_COMMENT_LENGTH) + '...';
		}

		return {
			pos: wordInfo.from,
			end: wordInfo.to,
			above: true,
			create() {
				const dom = document.createElement('div');
				dom.className = 'cm-schema-tooltip';
				dom.style.padding = '4px 8px';
				dom.style.maxWidth = '400px';
				dom.style.whiteSpace = 'pre-wrap';

				const typeSpan = document.createElement('span');
				typeSpan.style.color = 'var(--color-brand-primary, #6366f1)';
				typeSpan.style.fontWeight = '600';
				typeSpan.textContent = `${field.type}${field.repeated ? '[]' : ''}`;

				const commentSpan = document.createElement('span');
				commentSpan.textContent = ` — ${comment}`;

				dom.appendChild(typeSpan);
				dom.appendChild(commentSpan);

				return { dom };
			}
		};
	});
}
