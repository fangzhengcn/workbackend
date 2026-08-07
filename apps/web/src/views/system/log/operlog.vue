<!--
  操作日志：列表 + 筛选 + 详情抽屉 + 批量删除。

  只读 + 删除，没有新增/修改——日志由中间件自动写入，人工改动会破坏审计价值。
-->
<template>
  <div>
    <a-form layout="inline" class="search-form">
      <a-form-item label="操作模块">
        <a-input
          v-model:value="query.title"
          placeholder="如：用户管理"
          allow-clear
          @press-enter="search"
        />
      </a-form-item>
      <a-form-item label="操作人">
        <a-input v-model:value="query.operName" allow-clear @press-enter="search" />
      </a-form-item>
      <a-form-item label="类型">
        <a-select
          v-model:value="query.businessType"
          placeholder="全部"
          allow-clear
          style="width: 110px"
          :options="businessTypeOptions"
        />
      </a-form-item>
      <a-form-item label="状态">
        <a-select v-model:value="query.status" placeholder="全部" allow-clear style="width: 100px">
          <a-select-option :value="1">成功</a-select-option>
          <a-select-option :value="0">失败</a-select-option>
        </a-select>
      </a-form-item>
      <a-form-item label="时间">
        <a-range-picker v-model:value="dateRange" style="width: 240px" @change="onDateChange" />
      </a-form-item>
      <a-form-item>
        <a-space>
          <a-button type="primary" :loading="loading" @click="search">查询</a-button>
          <a-button @click="onReset">重置</a-button>
        </a-space>
      </a-form-item>
    </a-form>

    <div class="toolbar">
      <a-space>
        <a-button v-permission="Perms.operLog.export" :loading="exporting" @click="onExport">
          <DownloadOutlined /> 导出
        </a-button>
        <a-popconfirm
          :title="`确认删除选中的 ${selectedIds.length} 条日志？`"
          :disabled="selectedIds.length === 0"
          @confirm="onBatchDelete"
        >
          <a-button v-permission="Perms.operLog.remove" danger :disabled="selectedIds.length === 0">
            <DeleteOutlined /> 删除选中
          </a-button>
        </a-popconfirm>
        <a-popconfirm title="确认清空全部操作日志？此操作不可恢复" @confirm="onClean">
          <a-button v-permission="Perms.operLog.remove" danger>清空</a-button>
        </a-popconfirm>
      </a-space>
    </div>

    <a-table
      :columns="columns"
      :data-source="rows"
      :loading="loading"
      :pagination="pagination"
      :row-selection="rowSelection"
      row-key="id"
      size="middle"
      @change="onTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'businessType'">
          <a-tag :color="BUSINESS_COLORS[record.businessType]">
            {{ BUSINESS_LABELS[record.businessType] ?? '其他' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="record.status === 1 ? 'green' : 'red'">
            {{ record.status === 1 ? '成功' : '失败' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'costTime'">{{ record.costTime }} ms</template>
        <template v-else-if="column.key === 'action'">
          <a @click="onDetail(record as OperLogItem)">详情</a>
        </template>
      </template>
    </a-table>

    <!-- 详情抽屉：请求参数与响应体可能很长，用抽屉比弹窗更合适 -->
    <a-drawer v-model:open="detailOpen" title="操作日志详情" width="640px">
      <a-spin :spinning="detailLoading">
        <a-descriptions v-if="detail" :column="1" bordered size="small">
          <a-descriptions-item label="操作模块">{{ detail.title }}</a-descriptions-item>
          <a-descriptions-item label="业务类型">
            {{ BUSINESS_LABELS[detail.businessType] ?? '其他' }}
          </a-descriptions-item>
          <a-descriptions-item label="操作人">{{ detail.operName }}</a-descriptions-item>
          <a-descriptions-item label="操作 IP">{{ detail.operIp }}</a-descriptions-item>
          <a-descriptions-item label="请求方式">{{ detail.method }}</a-descriptions-item>
          <a-descriptions-item label="请求地址">{{ detail.requestUrl }}</a-descriptions-item>
          <a-descriptions-item label="状态">
            <a-tag :color="detail.status === 1 ? 'green' : 'red'">
              {{ detail.status === 1 ? '成功' : '失败' }}
            </a-tag>
          </a-descriptions-item>
          <a-descriptions-item v-if="detail.errorMsg" label="错误信息">
            <span class="danger">{{ detail.errorMsg }}</span>
          </a-descriptions-item>
          <a-descriptions-item label="耗时">{{ detail.costTime }} ms</a-descriptions-item>
          <a-descriptions-item label="操作时间">{{ detail.createdAt }}</a-descriptions-item>
          <a-descriptions-item label="请求参数">
            <!-- 敏感字段已在写入时脱敏（密码显示为 ***），这里原样展示 -->
            <pre class="code">{{ prettify(detail.requestParam) }}</pre>
          </a-descriptions-item>
          <a-descriptions-item label="返回结果">
            <pre class="code">{{ prettify(detail.jsonResult) }}</pre>
          </a-descriptions-item>
        </a-descriptions>
      </a-spin>
    </a-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { Dayjs } from 'dayjs'
import { DeleteOutlined, DownloadOutlined } from '@ant-design/icons-vue'
import { Perms } from '@workbackend/shared'
import {
  cleanOperLogs,
  deleteOperLogs,
  exportOperLogs,
  getOperLog,
  listOperLogs,
  type OperLogDetail,
  type OperLogItem,
  type OperLogQuery,
} from '@/api/log'
import { useTable } from '@/composables/useTable'

const { loading, rows, query, pagination, fetchData, search, reset, onTableChange } = useTable<
  OperLogItem,
  OperLogQuery
>(listOperLogs, {
  title: undefined,
  operName: undefined,
  businessType: undefined,
  status: undefined,
  beginTime: undefined,
  endTime: undefined,
})

const BUSINESS_LABELS: Record<number, string> = {
  0: '其他',
  1: '新增',
  2: '修改',
  3: '删除',
  4: '查询',
}

const BUSINESS_COLORS: Record<number, string> = {
  0: 'default',
  1: 'green',
  2: 'blue',
  3: 'red',
  4: 'cyan',
}

const businessTypeOptions = Object.entries(BUSINESS_LABELS).map(([value, label]) => ({
  label,
  value: Number(value),
}))

const columns = [
  { title: '操作模块', dataIndex: 'title', key: 'title', width: 120 },
  { title: '类型', key: 'businessType', width: 80 },
  { title: '操作人', dataIndex: 'operName', key: 'operName', width: 100 },
  { title: 'IP', dataIndex: 'operIp', key: 'operIp', width: 120 },
  { title: '请求地址', dataIndex: 'requestUrl', key: 'requestUrl', ellipsis: true },
  { title: '状态', key: 'status', width: 80 },
  { title: '耗时', key: 'costTime', width: 90 },
  { title: '操作时间', dataIndex: 'createdAt', key: 'createdAt', width: 170 },
  { title: '操作', key: 'action', width: 70, fixed: 'right' as const },
]

// ---- 时间区间 ----
// a-range-picker 的 change 事件同时给出 dayjs 对象与格式化字符串，用后者即可。
// 类型标成 [Dayjs, Dayjs] | undefined：组件的 v-model 只接受该形状，
// 用 unknown[] 过不了类型检查。
const dateRange = ref<[Dayjs, Dayjs]>()

function onDateChange(_dates: unknown, dateStrings: [string, string]): void {
  const [begin, end] = dateStrings
  query.beginTime = begin ? `${begin} 00:00:00` : undefined
  // 结束日期补到 23:59:59，否则当天产生的记录会被区间漏掉
  query.endTime = end ? `${end} 23:59:59` : undefined
}

function onReset(): void {
  dateRange.value = undefined
  reset()
}

// ---- 批量选择 ----
const selectedIds = ref<number[]>([])

const rowSelection = computed(() => ({
  selectedRowKeys: selectedIds.value,
  onChange: (keys: (string | number)[]) => {
    selectedIds.value = keys.map(Number)
  },
}))

async function onBatchDelete(): Promise<void> {
  try {
    await deleteOperLogs(selectedIds.value)
    message.success('删除成功')
    selectedIds.value = []
    fetchData()
  } catch {
    // 拦截器已提示（如超出单次 200 条上限）
  }
}

async function onClean(): Promise<void> {
  try {
    await cleanOperLogs()
    message.success('已清空')
    selectedIds.value = []
    fetchData()
  } catch {
    // 拦截器已提示
  }
}

const exporting = ref(false)

/** 导出当前筛选条件下的日志 */
async function onExport(): Promise<void> {
  exporting.value = true
  try {
    await exportOperLogs(query)
    message.success('导出成功')
  } catch {
    // download() 已按后端返回的原因提示（如超过导出上限）
  } finally {
    exporting.value = false
  }
}

// ---- 详情 ----
const detailOpen = ref(false)
const detailLoading = ref(false)
const detail = ref<OperLogDetail | null>(null)

async function onDetail(record: OperLogItem): Promise<void> {
  detail.value = null
  detailOpen.value = true
  detailLoading.value = true
  try {
    detail.value = await getOperLog(record.id)
  } catch {
    // 拦截器已提示
  } finally {
    detailLoading.value = false
  }
}

/** 尝试格式化 JSON，失败则原样返回（内容可能本就不是 JSON） */
function prettify(raw: string): string {
  if (!raw) return '—'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}
</script>

<style scoped>
.search-form {
  margin-bottom: 16px;
}

.toolbar {
  margin-bottom: 16px;
}

.danger {
  color: #ff4d4f;
}

.code {
  margin: 0;
  max-height: 240px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 12px;
}
</style>
