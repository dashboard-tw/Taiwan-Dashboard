// 無障礙：另開新視窗的連結要先告知使用者（WCAG 2.4.4）。
// 全站的 target="_blank" 連結都在 title 加上「（另開新視窗）」，並補一段只給報讀軟體的文字；
// 連結有 aria-label 時改加在 aria-label 裡，因為 aria-label 會蓋過連結內的文字。

const NOTE = '（另開新視窗）'
const MARK = 'data-new-window'

export function annotate(a: HTMLAnchorElement) {
  if (a.target !== '_blank') return
  const label = a.getAttribute('aria-label')
  if (label !== null) {
    if (!label.includes(NOTE)) a.setAttribute('aria-label', label + NOTE)
  } else if (!a.querySelector(`[${MARK}]`)) {
    const hint = document.createElement('span')
    hint.className = 'visually-hidden'
    hint.setAttribute(MARK, '')
    hint.textContent = NOTE
    a.append(hint)
  }
  const title = a.getAttribute('title')
  if (title === null || title === '') {
    const text = (label ?? a.textContent ?? '').replace(NOTE, '').trim()
    a.setAttribute('title', text ? text + NOTE : NOTE.slice(1, -1))
  } else if (!title.includes('另開新視窗')) {
    a.setAttribute('title', title + NOTE)
  }
}

function scan(root: ParentNode) {
  if (root instanceof HTMLAnchorElement) annotate(root)
  root.querySelectorAll?.<HTMLAnchorElement>('a[target="_blank"]').forEach(annotate)
}

export function watchNewWindowLinks(root: HTMLElement = document.body) {
  scan(root)
  new MutationObserver((records) => {
    for (const r of records) {
      if (r.type === 'attributes') {
        if (r.target instanceof HTMLAnchorElement) annotate(r.target)
        continue
      }
      r.addedNodes.forEach((n) => {
        if (n instanceof Element) scan(n)
      })
    }
  }).observe(root, {
    childList: true,
    subtree: true,
    attributes: true,
    attributeFilter: ['target', 'title', 'aria-label'],
  })
}
