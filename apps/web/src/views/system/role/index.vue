<!--
  角色管理：列表 + 搜索 + 增删改 + 菜单授权。

  数据范围的配置并入了 RoleFormModal（编辑角色时一并调整），
  不再单开一个弹窗——两者字段完全重合，拆开等于同一份表单维护两遍。
-->
<template>
  <div>
    <a-form layout="inline" class="search-form">
      <a-form-item label="角色名称">
        <a-input
          v-model:value="query.name"
          placeholder="支持模糊查询"
          allow-clear
          @press-enter="search"
        />
      </a-form-item>
      <a-form-item label="角色标识">
        <a-input
          v-model:value="query.code"
          placeholder="支持模糊查询"
          allow-clear
          @press-enter="search"
        />
      </a-form-item>
      <a-form-item label="状态">
        <a-select v-model:value="query.status" placeholder="全部" allow-clear style="width: 120px">
          <a-select-option :value="Status.Enabled">正常</a-select-option>
          <a-select-option :value="Status.Disabled">停用</a-select-option>
        </a-select>
      </a-form-item>
      <a-form-item>
        <a-space>
          <a-button type="primary" :loading="loading" @click="search">查询</a-button>
          <a-button @click="reset">重置</a-button>
        </a-space>
      </a-form-item>
    </a-form>

    <div class="toolbar">
      <a-button v-permission="Perms.role.add" type="primary" @click="onCreate">
        <PlusOutlined /> 新增
      </a-button>
    </div>

    <a-table
      :columns="columns"
      :data-source="rows"
      :loading="loading"
      :pagination="pagination"
      row-key="id"
      size="middle"
      @change="onTableChange"
    >
      <template #bodyCell="{ column, record }">
        <template v-if="column.key === 'dataScope'">
          {{ dataScopeLabel(record.dataScope) }}
        </template>
        <template v-else-if="column.key === 'status'">
          <a-tag :color="record.status === Status.Enabled ? 'green' : 'red'">
            {{ record.status === Status.Enabled ? '正常' : '停用' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <!--
            超级管理员角色受后端保护（不可改权限/停用/删除），
            前端一并隐藏这些入口，避免用户点了才收到 403。
          -->
          <a-space v-if="record.code !== SUPER_ADMIN_ROLE_CODE">
            <a v-permission="Perms.role.edit" @click="onEdit(record as RoleItem)">编辑</a>
            <a v-permission="Perms.role.assignMenu" @click="onAssign(record as RoleItem)">
              分配权限
            </a>
            <a-popconfirm title="确认删除该角色？" @confirm="onDelete(record as RoleItem)">
              <a v-permission="Perms.role.remove" class="danger">删除</a>
            </a-popconfirm>
          </a-space>
          <span v-else class="hint">系统内置</span>
        </template>
      </template>
    </a-table>

    <RoleFormModal v-model:open="formOpen" :record="current" @saved="fetchData" />
    <MenuTreeModal v-model:open="assignOpen" :record="current" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import {
  DataScope,
  Perms,
  Status,
  SUPER_ADMIN_ROLE_CODE,
  type RoleItem,
} from '@workbackend/shared'
import { deleteRole, listRoles, type RoleQuery } from '@/api/role'
import { useTable } from '@/composables/useTable'
import RoleFormModal from './RoleFormModal.vue'
import MenuTreeModal from './MenuTreeModal.vue'

const {
  loading,
  rows,
  query,
  pagination,
  fetchData,
  search,
  reset,
  onTableChange,
  refreshAfterRemove,
} = useTable<RoleItem, RoleQuery>(listRoles, {
  name: undefined,
  code: undefined,
  status: undefined,
})

const formOpen = ref(false)
const assignOpen = ref(false)
const current = ref<RoleItem | null>(null)

const DATA_SCOPE_LABELS: Record<number, string> = {
  [DataScope.All]: '全部数据',
  [DataScope.Custom]: '自定义',
  [DataScope.Dept]: '本部门',
  [DataScope.DeptTree]: '本部门及子部门',
  [DataScope.Self]: '仅本人',
}

function dataScopeLabel(scope: number): string {
  return DATA_SCOPE_LABELS[scope] ?? '未知'
}

const columns = [
  { title: '角色名称', dataIndex: 'name', key: 'name' },
  { title: '角色标识', dataIndex: 'code', key: 'code' },
  { title: '显示顺序', dataIndex: 'sort', key: 'sort', width: 100 },
  { title: '数据范围', key: 'dataScope', width: 140 },
  { title: '状态', key: 'status', width: 90 },
  { title: '备注', dataIndex: 'remark', key: 'remark', ellipsis: true },
  { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt' },
  { title: '操作', key: 'action', width: 200, fixed: 'right' as const },
]

function onCreate(): void {
  current.value = null
  formOpen.value = true
}

function onEdit(record: RoleItem): void {
  current.value = record
  formOpen.value = true
}

function onAssign(record: RoleItem): void {
  current.value = record
  assignOpen.value = true
}

async function onDelete(record: RoleItem): Promise<void> {
  try {
    await deleteRole(record.id)
    message.success('删除成功')
    refreshAfterRemove()
  } catch {
    // 拦截器已提示（如「该角色已分配给用户，请先解除分配」）
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

.hint {
  color: rgba(0, 0, 0, 0.45);
}
</style>
