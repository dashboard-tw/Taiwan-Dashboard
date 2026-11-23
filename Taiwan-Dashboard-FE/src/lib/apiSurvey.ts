import { TOPIC_TITLES } from './topics'

export const IDENTITIES: Record<string, string> = {
  individual: '一般民眾',
  student: '學生/研究人員',
  media: '新聞媒體',
  government: '政府機關',
  nonprofit: '非營利組織',
  company: '公司行號',
  other: '其他',
}

export const PURPOSES: Record<string, string> = {
  research: '學術研究',
  journalism: '新聞報導',
  app: '開發應用程式或網站',
  analysis: '資料分析',
  teaching: '教學',
  personal: '個人興趣',
  other: '其他',
}

export const PUSH: Record<string, string> = {
  yes: '希望新增',
  maybe: '可能會用到',
  no: '不需要',
  unanswered: '未回答',
}

export const TOPICS = TOPIC_TITLES

export function topicNames(codes: string[]): string {
  return codes.map((c) => TOPICS[c] ?? c).join('、')
}
