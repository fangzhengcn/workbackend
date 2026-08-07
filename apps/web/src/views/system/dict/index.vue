<!--
  字典管理：左侧字典类型、右侧该类型下的数据项。

  双栏布局而非两个独立页面：字典数据不能脱离类型存在，
  分成两页会让用户在「选类型」和「看数据」之间反复跳转。
-->
<template>
  <a-row :gutter="16">
    <!-- 左：字典类型 -->
    <a-col :span="10">
      <a-card size="small" title="字典类型">
        <template #extra>
          <a-button
            v-permission="Perms.dict.add"
            type="primary"
            size="small"
            @click="onCreateType"
          >
            <PlusOutlined /> 新增
          </a-button>
        </template>

        <a-input-search
          v-model:value="typeQuery.name"
          placeholder="按名称搜索"
          allow-clear
          class="search"
          @search="searchTypes"
        />

        <a-table
          :columns="typeColumns"
          :data-source="typeRows"
          :loading="typeLoading"
          :pagination="typePagination"
          :row-class-name="rowClassName"
          :custom-row="customTypeRow"
          row-key="id"
          size="small"
          @change="onTypeTableChange"
        >
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'status'">
              <a-tag :color="record.status === Status.Enabled ? 'green' : 'red'">
                {{ record.status === Status.Enabled ? '正常' : '停用' }}
              </a-tag>
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a v-permission="Perms.dict.edit" @click.stop="onEditType(record as DictTypeItem)">
                  编辑
                </a>
                <a-popconfirm
                  title="删除该类型会同时删除其下全部数据项，确认删除？"
                  @confirm="onDeleteType(record as DictTypeItem)"
                >
                  <a v-permission="Perms.dict.remove" class="danger" @click.stop>删除</a>
                </a-popconfirm>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-card>
    </a-col>

    <!-- 右：该类型下的数据项 -->
    <a-col :span="14">
      <a-card size="small">
        <template #title>
          字典数据
          <span v-if="currentType" class="subtitle">
            · {{ currentType.name }}（{{ currentType.type }}）
          </span>
        </template>
        <template #extra>
          <a-button
            v-permission="Perms.dict.add"
            type="primary"
            size="small"
            :disabled="!currentType"
            @click="onCreateData"
          >
            <PlusOutlined /> 新增
          </a-button>
        </template>

        <a-empty v-if="!currentType" description="请先在左侧选择一个字典类型" />

        <template v-else>
          <a-input-search
            v-model:value="dataQuery.label"
            placeholder="按标签搜索"
            allow-clear
            class="search"
            @search="searchData"
          />

          <a-table
            :columns="dataColumns"
            :data-source="dataRows"
            :loading="dataLoading"
            :pagination="dataPagination"
            row-key="id"
            size="small"
            @change="onDataTableChange"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'isDefault'">
                <a-tag v-if="record.isDefault === 1" color="blue">默认</a-tag>
                <span v-else class="muted">—</span>
              </template>
              <template v-else-if="column.key === 'status'">
                <a-tag :color="record.status === Status.Enabled ? 'green' : 'red'">
                  {{ record.status === Status.Enabled ? '正常' : '停用' }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'action'">
                <a-space>
                  <a v-permission="Perms.dict.edit" @click="onEditData(record as DictDataItem)">
                    编辑
                  </a>
                  <a-popconfirm title="确认删除该数据项？" @confirm="onDeleteData(record as DictDataItem)">
                    <a v-permission="Perms.dict.remove" class="danger">删除</a>
                  </a-popconfirm>
                </a-space>
              </template>
            </template>
          </a-table>
        </template>
      </a-card>
    </a-col>
  </a-row>

  <DictTypeModal v-model:open="typeModalOpen" :record="editingType" @saved="onTypeSaved" />
  <DictDataModal
    v-model:open="dataModalOpen"
    :record="editingData"
    :dict-type="currentType"
    @saved="fetchData"
  />
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import { Perms, Status } from '@workbackend/shared'
import {
  deleteDictData,
  deleteDictType,
  listDictData,
  listDictTypes,
  type DictDataItem,
  type DictDataQuery,
  type DictTypeItem,
  type DictTypeQuery,
} from '@/api/dict'
import { useTable } from '@/composables/useTable'
import DictTypeModal from './DictTypeModal.vue'
import DictDataModal from './DictDataModal.vue'

// 左侧类型列表
const {
  loading: typeLoading,
  rows: typeRows,
  query: typeQuery,
  pagination: typePagination,
  fetchData: fetchTypes,
  search: searchTypes,
  onTableChange: onTypeTableChange,
  refreshAfterRemove: refreshTypesAfterRemove,
} = useTable<DictTypeItem, DictTypeQuery>(listDictTypes, { name: undefined }, { pageSize: 10 })

// 右侧数据列表。dictType 由选中的类型注入，初始为空故不自动拉取。
const {
  loading: dataLoading,
  rows: dataRows,
  query: dataQuery,
  pagination: dataPagination,
  fetchData,
  search: searchData,
  onTableChange: onDataTableChange,
  refreshAfterRemove: refreshDataAfterRemove,
} = useTable<DictDataItem, DictDataQuery>(
  listDictData,
  { dictType: undefined, label: undefined },
  { immediate: false, pageSize: 10 },
)

/** 当前选中的字典类型 */
const currentType = ref<DictTypeItem | null>(null)

const typeModalOpen = ref(false)
const dataModalOpen = ref(false)
const editingType = ref<DictTypeItem | null>(null)
const editingData = ref<DictDataItem | null>(null)

const typeColumns = [
  { title: '字典名称', dataIndex: 'name', key: 'name' },
  { title: '类型标识', dataIndex: 'type', key: 'type', ellipsis: true },
  { title: '状态', key: 'status', width: 80 },
  { title: '操作', key: 'action', width: 110 },
]

const dataColumns = [
  { title: '标签', dataIndex: 'label', key: 'label' },
  { title: '键值', dataIndex: 'value', key: 'value' },
  { title: '排序', dataIndex: 'sort', key: 'sort', width: 70 },
  { title: '默认', key: 'isDefault', width: 70 },
  { title: '状态', key: 'status', width: 80 },
  { title: '操作', key: 'action', width: 110 },
]

/** 点击行即切换右侧数据；比额外放一个「查看」按钮更顺手 */
function customTypeRow(record: DictTypeItem) {
  return {
    onClick: () => {
      currentType.value = record
    },
    style: { cursor: 'pointer' },
  }
}

function rowClassName(record: DictTypeItem): string {
  return currentType.value?.id === record.id ? 'row-selected' : ''
}

// 切换类型后重新拉取右侧数据，并回到第一页
watch(currentType, (type) => {
  if (!type) return
  dataQuery.dictType = type.type
  dataQuery.label = undefined
  dataQuery.page = 1
  fetchData()
})

function onCreateType(): void {
  editingType.value = null
  typeModalOpen.value = true
}

function onEditType(record: DictTypeItem): void {
  editingType.value = record
  typeModalOpen.value = true
}

/**
 * 类型保存后要同步刷新右侧：类型标识可能被改过，
 * 而右侧的查询条件用的正是旧标识，不同步会查出空列表。
 */
function onTypeSaved(): void {
  const currentId = currentType.value?.id
  fetchTypes().then(() => {
    if (!currentId) return
    const updated = typeRows.value.find((item) => item.id === currentId)
    if (updated) {
      currentType.value = updated
      dataQuery.dictType = updated.type
      fetchData()
    }
  })
}

async function onDeleteType(record: DictTypeItem): Promise<void> {
  try {
    await deleteDictType(record.id)
    message.success('删除成功')
    // 删掉的正是当前选中项时，清空右侧避免展示已不存在类型的数据
    if (currentType.value?.id === record.id) {
      currentType.value = null
    }
    refreshTypesAfterRemove()
  } catch {
    // 拦截器已提示
  }
}

function onCreateData(): void {
  editingData.value = null
  dataModalOpen.value = true
}

function onEditData(record: DictDataItem): void {
  editingData.value = record
  dataModalOpen.value = true
}

async function onDeleteData(record: DictDataItem): Promise<void> {
  try {
    await deleteDictData(record.id)
    message.success('删除成功')
    refreshDataAfterRemove()
  } catch {
    // 拦截器已提示
  }
}
</script>

<style scoped>
.search {
  margin-bottom: 12px;
}

.subtitle {
  color: rgba(0, 0, 0, 0.45);
  font-weight: normal;
  font-size: 13px;
}

.danger {
  color: #ff4d4f;
}

.muted {
  color: rgba(0, 0, 0, 0.25);
}

:deep(.row-selected) {
  background-color: #e6f4ff;
}
</style>
