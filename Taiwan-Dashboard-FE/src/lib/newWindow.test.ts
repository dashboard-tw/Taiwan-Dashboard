// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { annotate } from './newWindow'

function link(html: string) {
  const box = document.createElement('div')
  box.innerHTML = html
  return box.querySelector('a')!
}

describe('annotate', () => {
  it('adds a title and a screen-reader hint to plain links', () => {
    const a = link('<a href="https://example.org" target="_blank">政府資料開放平臺</a>')
    annotate(a)
    expect(a.title).toBe('政府資料開放平臺（另開新視窗）')
    expect(a.textContent).toBe('政府資料開放平臺（另開新視窗）')
    annotate(a)
    expect(a.querySelectorAll('[data-new-window]')).toHaveLength(1)
  })

  it('puts the hint in aria-label when the link has one', () => {
    const a = link('<a href="#" target="_blank" aria-label="App Store" title="App Store">圖</a>')
    annotate(a)
    expect(a.getAttribute('aria-label')).toBe('App Store（另開新視窗）')
    expect(a.title).toBe('App Store（另開新視窗）')
    expect(a.querySelector('[data-new-window]')).toBeNull()
  })

  it('leaves same-window links alone', () => {
    const a = link('<a href="/about">關於</a>')
    annotate(a)
    expect(a.title).toBe('')
    expect(a.textContent).toBe('關於')
  })
})
