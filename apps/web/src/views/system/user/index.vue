<!--
  用户管理：本脚手架中「列表 + 搜索 + 增删改 + 按钮权限」的参考实现。
  其余管理页可照此结构展开——列表逻辑复用 useTable，弹窗各自拆成子组件。
-->
<template>
  <div>
    <a-form layout="inline" class="search-form">
      <a-form-item label="账号">
        <a-input
          v-model:value="query.username"
          placeholder="支持模糊查询"
          allow-clear
          @press-enter="search"
        />
      </a-form-item>
      <a-form-item label="手机号">
        <!-- 手机号加密存储，只能精确匹配（走 HMAC 盲索引），无法模糊搜索 -->
        <a-input
          v-model:value="query.phone"
          placeholder="需输入完整手机号"
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
      <a-space>
        <a-button v-permission="Perms.user.add" type="primary" @click="onCreate">
          <PlusOutlined /> 新增
        </a-button>
        <a-button v-permission="Perms.user.export" :loading="exporting" @click="onExport">
          <DownloadOutlined /> 导出
        </a-button>
      </a-space>
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
        <template v-if="column.key === 'status'">
          <a-tag :color="record.status === Status.Enabled ? 'green' : 'red'">
            {{ record.status === Status.Enabled ? '正常' : '停用' }}
          </a-tag>
        </template>
        <template v-else-if="column.key === 'roles'">
          <a-tag v-for="role in record.roles" :key="role.id" color="blue">{{ role.name }}</a-tag>
        </template>
        <template v-else-if="column.key === 'action'">
          <a-space>
            <a v-permission="Perms.user.edit" @click="onEdit(record as UserItem)">编辑</a>
            <a v-permission="Perms.user.resetPwd" @click="onResetPwd(record as UserItem)">
              重置密码
            </a>
            <!--
              admin（id=1）在后端被禁止删除与停用，前端一并隐藏入口，
              避免用户点了才收到 403。
            -->
            <a-popconfirm
              v-if="record.id !== ADMIN_USER_ID"
              title="确认删除该用户？"
              @confirm="onDelete(record as UserItem)"
            >
              <a v-permission="Perms.user.remove" class="danger">删除</a>
            </a-popconfirm>
          </a-space>
        </template>
      </template>
    </a-table>

    <UserFormModal v-model:open="formOpen" :record="current" @saved="fetchData" />
    <ResetPasswordModal v-model:open="resetOpen" :record="current" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import { DownloadOutlined, PlusOutlined } from '@ant-design/icons-vue'
import { Perms, Status, type UserItem } from '@workbackend/shared'
import { deleteUser, exportUsers, listUsers, type UserQuery } from '@/api/user'
import { useTable } from '@/composables/useTable'
import UserFormModal from './UserFormModal.vue'
import ResetPasswordModal from './ResetPasswordModal.vue'

/** 超级管理员用户 ID，后端 service.adminUserID 与此一致，禁止删除/停用 */
const ADMIN_USER_ID = 1

const { loading, rows, query, pagination, fetchData, search, reset, onTableChange, refreshAfterRemove } =
  useTable<UserItem, UserQuery>(listUsers, {
    username: undefined,
    phone: undefined,
    status: undefined,
  })

const formOpen = ref(false)
const resetOpen = ref(false)
/** 当前操作的行；新增时为 null */
const current = ref<UserItem | null>(null)

const columns = [
  { title: '账号', dataIndex: 'username', key: 'username' },
  { title: '昵称', dataIndex: 'nickname', key: 'nickname' },
  { title: '手机号', dataIndex: 'phone', key: 'phone' },
  { title: '部门', dataIndex: 'deptName', key: 'deptName' },
  { title: '角色', key: 'roles' },
  { title: '状态', key: 'status', width: 90 },
  { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt' },
  { title: '操作', key: 'action', width: 200, fixed: 'right' as const },
]

function onCreate(): void {
  current.value = null
  formOpen.value = true
}

function onEdit(record: UserItem): void {
  current.value = record
  formOpen.value = true
}

function onResetPwd(record: UserItem): void {
  current.value = record
  resetOpen.value = true
}

async function onDelete(record: UserItem): Promise<void> {
  try {
    await deleteUser(record.id)
    message.success('删除成功')
    refreshAfterRemove()
  } catch {
    // 拦截器已提示
  }
}

const exporting = ref(false)

/** 导出当前筛选条件下的数据；手机号/邮箱在文件里同样是脱敏值 */
async function onExport(): Promise<void> {
  exporting.value = true
  try {
    await exportUsers(query)
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

.danger {
  color: #ff4d4f;
}
</style>
