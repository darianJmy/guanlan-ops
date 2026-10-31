import { onScopeDispose } from 'vue'

export function useSearchDebounce(search, delay = 300) {
  let timer
  function cancel() {
    clearTimeout(timer)
  }
  function searchNow() {
    cancel()
    return search()
  }
  function scheduleSearch() {
    cancel()
    timer = setTimeout(searchNow, delay)
  }
  onScopeDispose(cancel)
  return { scheduleSearch, searchNow }
}
