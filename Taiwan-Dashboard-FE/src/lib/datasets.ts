import { ref } from 'vue'
import { getCatalog, type CatalogDataset } from '../api/catalog'

const datasets = ref<CatalogDataset[]>([])
let loading: Promise<void> | null = null

function load(): Promise<void> {
  if (!loading) {
    loading = getCatalog()
      .then((c) => {
        datasets.value = c.datasets
      })
      .catch(() => {
        loading = null
      })
  }
  return loading
}

export function useDatasets() {
  void load()
  return {
    datasets,
    datasetName(id: string): string {
      if (id === '_list') return '資料集清單'
      return datasets.value.find((d) => d.id === id)?.name ?? id
    },
  }
}
