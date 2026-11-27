export type Topic =
  | 'aqi'
  | 'reservoir'
  | 'power'
  | 'outage'
  | 'earthquake'
  | 'weather'
  | 'warning'
  | 'traffic'
  | 'resources'
  | 'population'
  | 'tourism'
  | 'rail'
  | 'bike'
  | 'safety'
  | 'family'
  | 'sustain'
  | 'agri'
  | 'food'
  | 'housing'
  | 'disease'
  | 'aviation'
  | 'economy'
  | 'animal'
  | 'fire'
  | 'forecast'
  | 'health'
  | 'ocean'
export type Scope = 'national' | 'county'
export type DataShape = 'by_county' | 'two_d' | 'three_d' | 'percent' | 'time' | 'map_legend'

export interface County {
  code: string
  name: string
  short_name: string
  name_en: string
  lon: number
  lat: number
}

export interface Source {
  dataset: string
  name: string
  agency: string
  license: string
  url: string
  status: 'pending' | 'ok' | 'stale' | 'error'
  last_observed_at: string | null
  update_freq_seconds: number
  last_ingested_at?: string | null
  stale_after_seconds?: number
}

export interface ComponentInfo {
  slug: string
  name: string
  topic: Topic
  short_desc: string
  long_desc: string
  use_case: string
  dataset_ids: string[]
  time_range: string
  chart_types: string[]
  colors: string[]
  unit: string
  decimals: number
  chart_map_link: unknown
  links: string[]
  scopes: Scope[]
  has_history: boolean
  has_map_layers?: boolean
  map_layers?: MapLayerInfo[]
  size?: 'normal' | 'wide'
  sources?: Source[]
}

export interface MapLayerInfo {
  id: number
  title: string
  layer_type: 'circle' | 'symbol' | 'fill' | 'line' | 'heatmap' | 'raster' | 'wind'
  source_type: 'county_boundary' | 'query'
  style: unknown
  popup_fields: unknown
}

export interface DashboardSummary {
  slug: string
  name: string
  icon: string
  description: string
}

export interface Dashboard extends DashboardSummary {
  components: ComponentInfo[]
}

export interface DataMeta {
  component: string
  scope: Scope
  data_shape: DataShape
  county?: string
  from?: string
  to?: string
  step?: string
  observed_at: string | null
  sources: Source[]
  categories?: string[]
  stale?: boolean
  as_of?: string
}

export interface ByCountyItem {
  county_code: string
  data: number | null
}

export interface Point {
  x: string
  y: number | null
}

export interface Series {
  name?: string
  data: Point[]
}

export interface NamedValues {
  name: string
  data: (number | null)[]
}

export type ChartData = ByCountyItem[] | Series[] | NamedValues[]

export interface ComponentData {
  data: ChartData
  meta: DataMeta
}
