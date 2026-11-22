import { describe, expect, it } from 'vitest'
import { detectLang, highlight } from './highlight'

describe('highlight', () => {
  it('escapes HTML', () => {
    expect(highlight('<b>&', 'text')).toBe('&lt;b&gt;&amp;')
    expect(highlight('echo "<script>"', 'shell')).toContain('&lt;script&gt;')
    expect(highlight('echo "<script>"', 'shell')).not.toContain('<script>')
  })

  it('colors JSON keys, strings and numbers', () => {
    const out = highlight('{ "aqi": 38, "status": "良好", "ok": true }', 'json')
    expect(out).toContain('<span class="tok-p">"aqi"</span>')
    expect(out).toContain('<span class="tok-n">38</span>')
    expect(out).toContain('<span class="tok-s">"良好"</span>')
    expect(out).toContain('<span class="tok-k">true</span>')
  })

  it('colors shell comments, flags and variables', () => {
    const out = highlight('# 取得 token\ncurl -X POST $URL', 'shell')
    expect(out).toContain('<span class="tok-c"># 取得 token</span>')
    expect(out).toContain('<span class="tok-f">-X</span>')
    expect(out).toContain('<span class="tok-v">$URL</span>')
  })

  it('keeps text unchanged apart from markup', () => {
    const code = 'const res = await fetch("https://a/b?x=1") // 註解'
    const out = highlight(code, 'js')
    expect(out.replace(/<[^>]+>/g, '')).toBe(code)
  })

  it('detects the language', () => {
    expect(detectLang('{ "a": 1 }')).toBe('json')
    expect(detectLang('curl "x"')).toBe('shell')
    expect(detectLang('x', 'Python')).toBe('python')
    expect(detectLang('a,b\n1,2')).toBe('text')
  })
})
