<!--
  部门管理：树形表格 + 增删改。

  与菜单管理同构：部门是树而非分页列表，一次取回全量在内存展开。
-->
<template>
  <div>
    <a-form layout="inline" class="search-form">
      <a-form-item label="部门名称">
        <a-input v-model:value="keyword" placeholder="按名称过滤" allow-clear />
      </a-form-item>
      <a-form-item>
        <a-space>
          <a-button type="primary" :loading="loading" @click="fetchTree">刷新</a-button>
          <a-button @click="expandAll(true)">展开全部</a-button>
          <a-button @click="expandAll(false)">收起全部</a-button>
        </a-space>
      </a-form-item>
    </a-form>

    <div class="toolbar">
      <a-button v-permission="Perms.dept.add" type="primary" @click="onCreate()">
        <PlusOutlined /> 新增
      </a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="filteredTree"
      :loading="loading"
      :pagination="false"
      :expanded-row-keys="expandedKeys"
      row-key="id"
      size="middle"
      @expanded-rows-change="onExpandedChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'status'">
          <a-tag :color="record.status === Status.Enabled ? 'green' : 'red'">
            {{ record.status === Status.Enabled ? '正常' : '停用' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a v-permission="Perms.dept.add" @click="onCreate(record as DeptNode)">新增子部门</a>
            <a v-permission="Perms.dept.edit" @click="onEdit(record as DeptNode)">编辑</a>
            <a-popconfirm title="确认删除该部门？" @confirm="onDelete(record as DeptNode)">
              <a v-permission="Perms.dept.remove" class="danger">删除</a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <DeptFormModal
      v-model:open="formOpen"
      :record="current"
      :default-parent-id="defaultParentId"
      :tree="tree"
      @saved="fetchTree"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import { Perms, Status, type DeptNode } from '@workbackend/shared'
import { deleteDept, getDeptTree } from '@/api/system'
import DeptFormModal from './DeptFormModal.vue'

const loading = ref(false)
const tree = ref<DeptNode[]>([])
const keyword = ref('')
const expandedKeys = ref<number[]>([])

const formOpen = ref(false)
const current = ref<DeptNode | null>(null)
const defaultParentId = ref<number | undefined>(undefined)

const columns = [
  { title: '部门名称', dataIndex: 'name', key: 'name', width: 240 },
  { title: '负责人', dataIndex: 'leader', key: 'leader' },
  { title: '联系电话', dataIndex: 'phone', key: 'phone' },
  { title: '排序', dataIndex: 'sort', key: 'sort', width: 80 },
  { title: '状态', key: 'status', width: 90 },
  { title: '操作', key: 'action', width: 240, fixed: 'right' as const },
]

function collectIds(nodes: DeptNode[]): number[] {
  const ids: number[] = []
  const walk = (list: DeptNode[]) => {
    for (const node of list) {
      ids.push(node.id)
      if (node.children?.length) walk(node.children)
    }
  }
  walk(nodes)
  return ids
}

/** 按名称过滤，保留命中节点的祖先链，否则命中的子节点会整条不可见 */
const filteredTree = computed<DeptNode[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return tree.value

  const filter = (nodes: DeptNode[]): DeptNode[] => {
    const result: DeptNode[] = []
    for (const node of nodes) {
      const children = node.children?.length ? filter(node.children) : []
      const hit = node.name.toLowerCase().includes(kw)
      if (hit || children.length > 0) {
        result.push({ ...node, children: children.length ? children : undefined })
      }
    }
    return result
  }
  return filter(tree.value)
})

async function fetchTree(): Promise<void> {
  loading.value = true
  try {
    tree.value = (await getDeptTree()) ?? []
    // 部门层级通常不深，默认全展开更实用
    expandedKeys.value = collectIds(tree.value)
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}

function expandAll(open: boolean): void {
  expandedKeys.value = open ? collectIds(tree.value) : []
}

function onExpandedChange(keys: (string | number)[]): void {
  expandedKeys.value = keys.map(Number)
}

function onCreate(parent?: DeptNode): void {
  current.value = null
  defaultParentId.value = parent?.id
  formOpen.value = true
}

function onEdit(record: DeptNode): void {
  current.value = record
  defaultParentId.value = undefined
  formOpen.value = true
}

async function onDelete(record: DeptNode): Promise<void> {
  try {
    await deleteDept(record.id)
    message.success('删除成功')
    fetchTree()
  } catch {
    // 拦截器已提示（如「该部门下还有用户」）
  }
}

onMounted(fetchTree)
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
</style>
