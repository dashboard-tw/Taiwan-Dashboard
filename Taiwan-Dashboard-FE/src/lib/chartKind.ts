export function chartKind(type: string | undefined): 'bar' | 'donut' | 'line' | 'map' {
  switch (type) {
    case 'CountyMapChart':
    case 'ImageChart':
      return 'map'
    case 'DonutChart':
    case 'TreemapChart':
      return 'donut'
    case 'TimelineSeparateChart':
    case 'TimelineStackedChart':
      return 'line'
    default:
      return 'bar'
  }
}
