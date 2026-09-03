import { isEmptyMarkdown } from './isEmptyMarkdown';

describe('isEmptyMarkdown', () => {
  it.each([
    {
      name: 'empty markdown cell',
      content: { kind: 'Markdown' as const, spec: { text: '' } },
      expected: true,
    },
    {
      name: 'markdown with only whitespace is not empty',
      content: { kind: 'Markdown' as const, spec: { text: ' ' } },
      expected: false,
    },
    {
      name: 'markdown with text',
      content: { kind: 'Markdown' as const, spec: { text: 'hello' } },
      expected: false,
    },
    {
      name: 'undefined (panel / collapsed cell with no content)',
      content: undefined,
      expected: false,
    },
    {
      name: 'empty code cell',
      content: { kind: 'Code' as const, spec: { language: 'promql', code: '' } },
      expected: false,
    },
    {
      name: 'code cell with source',
      content: { kind: 'Code' as const, spec: { language: 'promql', code: 'up' } },
      expected: false,
    },
  ])('returns $expected for $name', ({ content, expected }) => {
    expect(isEmptyMarkdown(content)).toBe(expected);
  });
});
