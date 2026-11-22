export interface DeviceName {
  browser: string
  os: string
  icon: 'smartphone' | 'tablet' | 'computer' | 'devices'
}

const BROWSERS: [RegExp, string][] = [
  [/Edg(e|A|iOS)?\//, 'Edge'],
  [/OPR\/|Opera/, 'Opera'],
  [/SamsungBrowser\//, 'Samsung Internet'],
  [/Line\//, 'LINE'],
  [/FBAN|FBAV/, 'Facebook'],
  [/Firefox\/|FxiOS\//, 'Firefox'],
  [/Chrome\/|CriOS\//, 'Chrome'],
  [/Safari\//, 'Safari'],
]

const SYSTEMS: [RegExp, string][] = [
  [/iPad/, 'iPadOS'],
  [/iPhone|iPod/, 'iOS'],
  [/Android/, 'Android'],
  [/CrOS/, 'ChromeOS'],
  [/Windows/, 'Windows'],
  [/Mac OS X|Macintosh/, 'macOS'],
  [/Linux/, 'Linux'],
]

function first(ua: string, table: [RegExp, string][]): string {
  return table.find(([re]) => re.test(ua))?.[1] ?? '未知'
}

export function describeUserAgent(ua: string): DeviceName {
  const browser = first(ua, BROWSERS)
  const os = first(ua, SYSTEMS)
  let icon: DeviceName['icon'] = 'devices'
  if (os === 'iPadOS' || (os === 'Android' && !/Mobile/.test(ua))) icon = 'tablet'
  else if (os === 'iOS' || os === 'Android') icon = 'smartphone'
  else if (os !== '未知') icon = 'computer'
  return { browser, os, icon }
}
