export type Lang = 'shell' | 'json' | 'python' | 'js' | 'text'

const KEYWORDS: Record<Lang, string[]> = {
  shell: ['curl'],
  json: ['true', 'false', 'null'],
  python: [
    'import',
    'from',
    'as',
    'def',
    'return',
    'if',
    'else',
    'for',
    'in',
    'True',
    'False',
    'None',
    'print',
  ],
  js: [
    'const',
    'let',
    'await',
    'async',
    'new',
    'return',
    'import',
    'from',
    'true',
    'false',
    'null',
    'console',
  ],
  text: [],
}

const escape = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

function pattern(lang: Lang): RegExp {
  const parts: string[] = []
  if (lang === 'shell' || lang === 'python') parts.push('(?<c>#[^\\n]*)')
  if (lang === 'js') parts.push('(?<c>\\/\\/[^\\n]*)')
  if (lang === 'json') parts.push('(?<p>"[^"\\n]*"(?=\\s*:))')
  const dq = '"(?:\\\\.|[^"\\\\\\n])*"'
  const sq = "'(?:\\\\.|[^'\\\\\\n])*'"
  const bq = '`(?:\\\\.|[^`\\\\])*`'
  const strings = [lang === 'python' ? `f?${dq}` : dq, sq]
  if (lang === 'js') strings.push(bq)
  parts.push(`(?<s>${strings.join('|')})`)
  if (lang === 'shell') {
    parts.push('(?<f>(?<=\\s)--?[A-Za-z][\\w-]*)')
    parts.push('(?<v>\\$[A-Za-z_]\\w*)')
  }
  parts.push('(?<n>(?<![\\w.])-?\\d+(?:\\.\\d+)?(?![\\w.]))')
  const kw = KEYWORDS[lang]
  if (kw.length) parts.push(`(?<k>\\b(?:${kw.join('|')})\\b)`)
  if (lang === 'python' || lang === 'js') parts.push('(?<fn>\\b[A-Za-z_]\\w*(?=\\())')
  return new RegExp(parts.join('|'), 'gm')
}

const CACHE = new Map<Lang, RegExp>()
/** Capture group names, which are also the CSS class suffixes. */
const GROUPS = ['c', 'p', 's', 'f', 'v', 'n', 'k', 'fn']

export function highlight(code: string, lang: Lang): string {
  if (lang === 'text') return escape(code)
  let re = CACHE.get(lang)
  if (!re) CACHE.set(lang, (re = pattern(lang)))
  let out = ''
  let last = 0
  for (const m of code.matchAll(re)) {
    const g = m.groups ?? {}
    const cls = GROUPS.find((k) => g[k] !== undefined)
    if (!cls || m.index === undefined) continue
    out += escape(code.slice(last, m.index)) + `<span class="tok-${cls}">${escape(m[0])}</span>`
    last = m.index + m[0].length
  }
  return out + escape(code.slice(last))
}

/** Guesses the language from the tab name, then from the code itself. */
export function detectLang(code: string, tab?: string): Lang {
  const t = (tab ?? '').toLowerCase()
  if (t === 'python') return 'python'
  if (t === 'javascript') return 'js'
  if (t === 'json') return 'json'
  const s = code.trimStart()
  if (s.startsWith('{') || s.startsWith('[')) return 'json'
  if (/^(#|curl\b)/m.test(s)) return 'shell'
  return 'text'
}
