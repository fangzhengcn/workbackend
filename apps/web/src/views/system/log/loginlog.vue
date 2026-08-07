<!--
  登录日志：列表 + 筛选 + 批量删除。

  结构与 operlog.vue 同构，但没有详情抽屉——登录日志的字段都很短，
  列表里一次展示得完，再点开一层反而多余。
-->
<template>
  <div>
    <a-form layout="inline" class="search-form">
      <a-form-item label="登录账号">
        <a-input
          v-model:value="query.username"
          placeholder="支持模糊查询"
          allow-clear
          @press-enter="search"
        />
      </a-form-item>
      <a-form-item label="登录 IP">
        <a-input v-model:value="query.ipaddr" allow-clear @press-enter="search" />
      </a-form-item>
      <a-form-item label="状态">
        <a-select v-model:value="query.status" placeholder="全部" allow-clear style="width: 110px">
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
        <a-button v-permission="Perms.loginLog.export" :loading="exporting" @click="onExport">
          <DownloadOutlined /> 导出
        </a-button>
        <a-popconfirm
          :title="`确认删除选中的 ${selectedIds.length} 条日志？`"
          :disabled="selectedIds.length === 0"
          @confirm="onBatchDelete"
        >
          <a-button
            v-permission="Perms.loginLog.remove"
            danger
            :disabled="selectedIds.length === 0"
          >
            <DeleteOutlined /> 删除选中
          </a-button>
        </a-popconfirm>
        <a-popconfirm title="确认清空全部登录日志？此操作不可恢复" @confirm="onClean">
          <a-button v-permission="Perms.loginLog.remove" danger>清空</a-button>
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
        <template v-if="column.key === 'status'">
          <a-tag :color="record.status === 1 ? 'green' : 'red'">
            {{ record.status === 1 ? '成功' : '失败' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'browser'">
          <!-- 存的是原始 User-Agent，通常很长；悬浮看全文 -->
          <a-tooltip :title="record.browser">
            <span class="ua">{{ record.browser || '—' }}</span>
          </a-tooltip>
        </template>
        <template v-else-if="column.key === 'location'">
          {{ record.location || '—' }}
        </template>
      </template>
    </a-table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { message } from 'ant-design-vue'
import type { Dayjs } from 'dayjs'
import { DeleteOutlined, DownloadOutlined } from '@ant-design/icons-vue'
import { Perms } from '@workbackend/shared'
import {
  cleanLoginLogs,
  deleteLoginLogs,
  exportLoginLogs,
  listLoginLogs,
  type LoginLogItem,
  type LoginLogQuery,
} from '@/api/log'
import { useTable } from '@/composables/useTable'

const { loading, rows, query, pagination, fetchData, search, reset, onTableChange } = useTable<
  LoginLogItem,
  LoginLogQuery
>(listLoginLogs, {
  username: undefined,
  ipaddr: undefined,
  status: undefined,
  beginTime: undefined,
  endTime: undefined,
})

const columns = [
  { title: '登录账号', dataIndex: 'username', key: 'username', width: 130 },
  { title: '登录 IP', dataIndex: 'ipaddr', key: 'ipaddr', width: 140 },
  { title: '登录地点', key: 'location', width: 120 },
  { title: '浏览器', key: 'browser', ellipsis: true },
  { title: '状态', key: 'status', width: 90 },
  { title: '提示消息', dataIndex: 'msg', key: 'msg', width: 160 },
  { title: '登录时间', dataIndex: 'loginTime', key: 'loginTime', width: 180 },
]

// ---- 时间区间 ----
// 类型须为 [Dayjs, Dayjs]：组件 v-model 只接受该形状，用 unknown[] 过不了类型检查。
const dateRange = ref<[Dayjs, Dayjs]>()

function onDateChange(_dates: unknown, dateStrings: [string, string]): void {
  const [begin, end] = dateStrings
  query.beginTime = begin ? `${begin} 00:00:00` : undefined
  // 结束日期补到 23:59:59，否则当天的登录记录会被区间漏掉
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
    await deleteLoginLogs(selectedIds.value)
    message.success('删除成功')
    selectedIds.value = []
    fetchData()
  } catch {
    // 拦截器已提示（如超出单次 200 条上限）
  }
}

async function onClean(): Promise<void> {
  try {
    await cleanLoginLogs()
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
    await exportLoginLogs(query)
    message.success('导出成功')
  } catch {
    // download() 已按后端返回的原因提示（如超过导出上限）
  } finally {
    exporting.value = false
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

.ua {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}
</style>
