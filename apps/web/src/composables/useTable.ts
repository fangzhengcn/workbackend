/**
 * 列表页通用逻辑：查询条件 + 分页 + 加载态 + 拉取。
 *
 * 抽出来的原因：用户/角色/菜单/部门/字典/日志七个管理页的这套逻辑完全同构，
 * 内联在每个页面里会复制七份，改分页默认值或错误处理就得改七处。
 *
 * 不负责的部分：表格列定义、弹窗、行操作——这些各页差异大，抽象反而碍事。
 */
import { onMounted, reactive, ref, type Ref } from 'vue'
import type { TablePaginationConfig } from 'ant-design-vue'
import type { PageResult } from '@workbackend/shared'

/** 分页查询函数签名，与 api 层的 listXxx 一致 */
type Fetcher<T, Q> = (query: Q) => Promise<PageResult<T>>

export interface UseTableOptions {
  /** 是否在 onMounted 时自动拉取，默认 true */
  immediate?: boolean
  /** 每页条数，默认 10 */
  pageSize?: number
}

export function useTable<T, Q extends { page?: number; size?: number }>(
  fetcher: Fetcher<T, Q>,
  initialQuery: Q,
  options: UseTableOptions = {},
) {
  const { immediate = true, pageSize = 10 } = options

  const loading = ref(false)
  // Ref<T[]>：泛型经过 ref() 会被推断成 UnwrapRef，显式标注避免调用方拿到
  // 一个不像数组的类型（模板里传给 a-table 的 data-source 会报类型不符）。
  const rows = ref([]) as Ref<T[]>

  const query = reactive({ ...initialQuery, page: 1, size: pageSize }) as Q

  // 保留一份初始值副本用于重置。不能直接复用 initialQuery——
  // query 是它的浅拷贝，reactive 后若引用同一对象会被后续赋值污染。
  const defaults = { ...initialQuery }

  const pagination = reactive<TablePaginationConfig>({
    current: 1,
    pageSize,
    total: 0,
    showSizeChanger: true,
    showTotal: (total: number) => `共 ${total} 条`,
  })

  async function fetchData(): Promise<void> {
    loading.value = true
    try {
      const result = await fetcher(query)
      rows.value = result.list ?? []
      pagination.total = result.total
      pagination.current = result.page
      pagination.pageSize = result.size
    } catch {
      // 错误提示已由 Axios 响应拦截器统一处理，这里只需结束加载态
    } finally {
      loading.value = false
    }
  }

  /** 条件变更后从第一页重新查询 */
  function search(): void {
    query.page = 1
    fetchData()
  }

  /** 清空查询条件并回到第一页 */
  function reset(): void {
    Object.assign(query, defaults, { page: 1, size: pageSize })
    fetchData()
  }

  function onTableChange(page: TablePaginationConfig): void {
    query.page = page.current ?? 1
    query.size = page.pageSize ?? pageSize
    fetchData()
  }

  /**
   * 删除后刷新。若当前页被删空且不是第一页，自动回退一页——
   * 否则用户会停在一个空列表上，误以为数据全没了。
   */
  function refreshAfterRemove(removedCount = 1): void {
    const remaining = rows.value.length - removedCount
    if (remaining <= 0 && (query.page ?? 1) > 1) {
      query.page = (query.page ?? 1) - 1
    }
    fetchData()
  }

  if (immediate) {
    onMounted(fetchData)
  }

  return {
    loading,
    rows,
    query,
    pagination,
    fetchData,
    search,
    reset,
    onTableChange,
    refreshAfterRemove,
  }
}
