<!--
  菜单管理：树形表格 + 增删改。

  不用 useTable：菜单是一棵树而非分页列表，一次性取回全量在内存里展开
  （设计文档「技术难点提示 3」：树在内存组装，不递归查库）。
-->
<template>
  <div>
    <a-form layout="inline" class="search-form">
      <a-form-item label="菜单名称">
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
      <a-button v-permission="Perms.menu.add" type="primary" @click="onCreate()">
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
        <template v-if="column.key === 'type'">
          <a-tag :color="TYPE_COLORS[record.type]">{{ TYPE_LABELS[record.type] }}</a-tag>
        </template>
        <template v-else-if="column.key === 'visible'">
          <span v-if="record.type === MenuType.Button" class="muted">—</span>
          <a-tag v-else :color="record.visible === Status.Enabled ? 'green' : 'orange'">
            {{ record.visible === Status.Enabled ? '显示' : '隐藏' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="record.status === Status.Enabled ? 'green' : 'red'">
            {{ record.status === Status.Enabled ? '正常' : '停用' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <!-- 按钮不能作为上级，故不提供「新增子菜单」入口 -->
            <a
              v-if="record.type !== MenuType.Button"
              v-permission="Perms.menu.add"
              @click="onCreate(record as MenuNode)"
            >
              新增子菜单
            </a>
            <a v-permission="Perms.menu.edit" @click="onEdit(record as MenuNode)">编辑</a>
            <a-popconfirm
              title="确认删除该菜单？删除后相关角色将失去此权限"
              @confirm="onDelete(record as MenuNode)"
            >
              <a v-permission="Perms.menu.remove" class="danger">删除</a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <MenuFormModal
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
import { MenuType, Perms, Status, type MenuNode } from '@workbackend/shared'
import { deleteMenu, getMenuTree } from '@/api/system'
import MenuFormModal from './MenuFormModal.vue'

const loading = ref(false)
const tree = ref<MenuNode[]>([])
const keyword = ref('')
const expandedKeys = ref<number[]>([])

const formOpen = ref(false)
const current = ref<MenuNode | null>(null)
const defaultParentId = ref<number | undefined>(undefined)

const TYPE_LABELS: Record<number, string> = {
  [MenuType.Dir]: '目录',
  [MenuType.Menu]: '菜单',
  [MenuType.Button]: '按钮',
}

const TYPE_COLORS: Record<number, string> = {
  [MenuType.Dir]: 'purple',
  [MenuType.Menu]: 'blue',
  [MenuType.Button]: 'default',
}

const columns = [
  { title: '菜单名称', dataIndex: 'name', key: 'name', width: 220 },
  { title: '类型', key: 'type', width: 80 },
  { title: '路由地址', dataIndex: 'path', key: 'path' },
  { title: '组件路径', dataIndex: 'component', key: 'component', ellipsis: true },
  { title: '权限标识', dataIndex: 'perms', key: 'perms', ellipsis: true },
  { title: '排序', dataIndex: 'sort', key: 'sort', width: 70 },
  { title: '侧边栏', key: 'visible', width: 90 },
  { title: '状态', key: 'status', width: 90 },
  { title: '操作', key: 'action', width: 240, fixed: 'right' as const },
]

/** 收集全部节点 ID */
function collectIds(nodes: MenuNode[]): number[] {
  const ids: number[] = []
  const walk = (list: MenuNode[]) => {
    for (const node of list) {
      ids.push(node.id)
      if (node.children?.length) walk(node.children)
    }
  }
  walk(nodes)
  return ids
}

/**
 * 按名称过滤，但保留命中节点的祖先链——
 * 否则匹配到的子节点会因为父节点被过滤掉而整条不可见。
 */
const filteredTree = computed<MenuNode[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return tree.value

  const filter = (nodes: MenuNode[]): MenuNode[] => {
    const result: MenuNode[] = []
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
    tree.value = (await getMenuTree()) ?? []
    // 默认展开一层即可，全展开在菜单多时会很长
    expandedKeys.value = tree.value.map((node) => node.id)
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

function onCreate(parent?: MenuNode): void {
  current.value = null
  defaultParentId.value = parent?.id
  formOpen.value = true
}

function onEdit(record: MenuNode): void {
  current.value = record
  defaultParentId.value = undefined
  formOpen.value = true
}

async function onDelete(record: MenuNode): Promise<void> {
  try {
    await deleteMenu(record.id)
    message.success('删除成功')
    fetchTree()
  } catch {
    // 拦截器已提示（如「该菜单下还有子菜单」）
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

.muted {
  color: rgba(0, 0, 0, 0.25);
}
</style>
