import { getCached } from './client'

export interface CatalogField {
  name: string
  type: 'string' | 'integer' | 'number' | 'boolean' | 'timestamp' | 'date'
  unit?: string
  description: string
}

export interface CatalogTopic {
  id: string
  name: string
}

export interface CatalogDataset {
  id: string
  name: string
  description: string
  agency: string
  license: string
  source_url: string
  status: 'pending' | 'ok' | 'stale' | 'error'
  update_freq_seconds: number
  last_observed_at: string | null
  has_sites: boolean
  has_county: boolean
  topic: string
  has_history: boolean
  latest_paged: boolean
  has_downloads: boolean
  fields: CatalogField[]
  latest_fields: CatalogField[]
  site_fields: CatalogField[]
}

export interface Catalog {
  topics: CatalogTopic[]
  datasets: CatalogDataset[]
  page_size: number
  max_range_days: number
}

export async function getCatalog(): Promise<Catalog> {
  return (await getCached<Catalog>('/catalog')).data
}
