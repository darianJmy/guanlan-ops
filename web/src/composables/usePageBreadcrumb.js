import { onBeforeUnmount, shallowRef, watchEffect } from 'vue'
import { useRoute } from 'vue-router'

export const pageBreadcrumb = shallowRef(null)

export function usePageBreadcrumb(getTitle) {
  const route = useRoute()
  const owner = Symbol('page-breadcrumb')
  watchEffect(() => {
    pageBreadcrumb.value = { owner, path: route.path, title: getTitle() }
  })
  onBeforeUnmount(() => {
    if (pageBreadcrumb.value?.owner === owner) pageBreadcrumb.value = null
  })
}
