<!--
  用户新增/编辑弹窗。

  新增与编辑合用一个组件：两者字段高度重叠，拆成两个文件会让校验规则与
  部门/角色下拉的加载逻辑重复一遍。差异用 isEdit 分支处理。
-->
<template>
  <a-modal
    :open="open"
    :title="isEdit ? '编辑用户' : '新增用户'"
    :confirm-loading="submitting"
    width="600px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-form ref="formRef" :model="form" :rules="rules" :label-col="{ span: 5 }">
      <a-form-item label="账号" name="username">
        <!-- 账号是登录凭据与审计主键，创建后不允许改 -->
        <a-input
          v-model:value="form.username"
          :disabled="isEdit"
          placeholder="2-64 位，登录用"
          allow-clear
        />
      </a-form-item>

      <a-form-item v-if="!isEdit" label="密码" name="password">
        <a-input-password v-model:value="form.password" placeholder="6-64 位" />
      </a-form-item>

      <a-form-item label="昵称" name="nickname">
        <a-input v-model:value="form.nickname" placeholder="选填" allow-clear />
      </a-form-item>

      <a-form-item label="手机号" name="phone">
        <!--
          编辑时留空表示不修改。
          列表与详情接口返回的都是脱敏值（138****8000），若把它回填进输入框再提交，
          会把脱敏串当明文加密入库，真实手机号就被破坏了。
          故编辑态一律留空，由后端的 *string nil 语义保持原值。
        -->
        <a-input
          v-model:value="form.phone"
          :placeholder="isEdit ? '留空表示不修改' : '11 位手机号，选填'"
          allow-clear
        />
      </a-form-item>

      <a-form-item label="邮箱" name="email">
        <a-input
          v-model:value="form.email"
          :placeholder="isEdit ? '留空表示不修改' : '选填'"
          allow-clear
        />
      </a-form-item>

      <a-form-item label="性别" name="gender">
        <a-radio-group v-model:value="form.gender">
          <a-radio :value="Gender.Unknown">未知</a-radio>
          <a-radio :value="Gender.Male">男</a-radio>
          <a-radio :value="Gender.Female">女</a-radio>
        </a-radio-group>
      </a-form-item>

      <a-form-item label="部门" name="deptId">
        <a-tree-select
          v-model:value="form.deptId"
          :tree-data="deptTree"
          :field-names="{ label: 'name', value: 'id', children: 'children' }"
          placeholder="选填"
          allow-clear
          tree-default-expand-all
        />
      </a-form-item>

      <a-form-item label="角色" name="roleIds">
        <a-select
          v-model:value="form.roleIds"
          mode="multiple"
          placeholder="可多选"
          :options="roleOptions"
          allow-clear
        />
      </a-form-item>

      <a-form-item label="状态" name="status">
        <a-radio-group v-model:value="form.status">
          <a-radio :value="Status.Enabled">正常</a-radio>
          <a-radio :value="Status.Disabled">停用</a-radio>
        </a-radio-group>
      </a-form-item>

      <a-form-item label="备注" name="remark">
        <a-textarea v-model:value="form.remark" :rows="2" placeholder="选填" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import { Gender, Status, type DeptNode, type RoleItem, type UserItem } from '@workbackend/shared'
import { createUser, updateUser, type UserPayload } from '@/api/user'
import { listAllRoles } from '@/api/role'
import { getDeptTree } from '@/api/system'

const props = defineProps<{
  open: boolean
  /** 传入则为编辑态，null 为新增 */
  record: UserItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const deptTree = ref<DeptNode[]>([])
const roles = ref<RoleItem[]>([])

const isEdit = computed(() => props.record !== null)

const roleOptions = computed(() =>
  roles.value.map((role) => ({ label: role.name, value: role.id })),
)

function emptyForm(): UserPayload {
  return {
    username: '',
    password: '',
    nickname: '',
    phone: '',
    email: '',
    gender: Gender.Unknown,
    deptId: undefined,
    status: Status.Enabled,
    remark: '',
    roleIds: [],
  }
}

const form = reactive<UserPayload>(emptyForm())

/*
 * 校验规则随模式变化：编辑态不渲染密码框，若仍挂 required 规则，
 * validate() 会因为一个用户看不见的字段而失败，表现为「点确定没反应」。
 */
const rules = computed<Record<string, Rule[]>>(() => {
  const base: Record<string, Rule[]> = {
    // 手机号与邮箱都是选填；async-validator 对非 required 的空值会跳过规则，
    // 所以填了才校验格式，留空（编辑态表示不修改）不会被拦。
    phone: [{ pattern: /^1\d{10}$/, message: '请输入 11 位手机号' }],
    email: [{ type: 'email', message: '邮箱格式不正确' }],
  }
  if (!isEdit.value) {
    base.username = [{ required: true, min: 2, max: 64, message: '账号需 2-64 位' }]
    base.password = [{ required: true, min: 6, max: 64, message: '密码需 6-64 位' }]
  }
  return base
})

/**
 * 每次打开弹窗都重新拉取角色与部门。
 *
 * 不做「只拉一次」的缓存：角色和部门是在别的页面维护的，
 * 用户在角色管理里新建一个角色后回到本页，缓存会让下拉框永远看不到它
 * （组件实例还在内存中，除非刷新整页）。
 * 这两个接口都很轻（不分页、无关联查询），每次开弹窗多一次请求完全值得。
 */
async function loadOptions(): Promise<void> {
  // 并发拉取，避免串行等待两个往返。
  // 用 allSettled：任一失败不应阻断另一个，用户至少还能选到能选的那部分。
  await Promise.allSettled([
    listAllRoles().then((data) => {
      roles.value = data ?? []
    }),
    getDeptTree().then((data) => {
      deptTree.value = data ?? []
    }),
  ])
  // 失败提示已由 Axios 拦截器统一处理
}

watch(
  () => props.open,
  async (opened) => {
    if (!opened) return
    await loadOptions()

    Object.assign(form, emptyForm())
    if (props.record) {
      const r = props.record
      Object.assign(form, {
        username: r.username,
        nickname: r.nickname,
        gender: r.gender,
        deptId: r.deptId ?? undefined,
        status: r.status,
        remark: r.remark,
        roleIds: r.roles?.map((role) => role.id) ?? [],
        // phone/email 故意不回填，见模板中的说明
        phone: '',
        email: '',
      })
    }
    formRef.value?.clearValidate()
  },
)

/**
 * 组装提交载荷。
 *
 * 手机号/邮箱只在填了值时才带上：编辑态下留空意味着「不修改」，
 * 后端 UpdateUserRequest 用 *string 的 nil 表达这一语义。
 * 若发空字符串，会被当成「显式清空」而覆盖掉原有密文。
 */
function buildPayload(): UserPayload {
  const payload: UserPayload = {
    nickname: form.nickname,
    gender: form.gender,
    deptId: form.deptId ?? null,
    status: form.status,
    remark: form.remark,
    roleIds: form.roleIds,
  }

  if (form.phone) payload.phone = form.phone
  if (form.email) payload.email = form.email

  if (!isEdit.value) {
    payload.username = form.username
    payload.password = form.password
  }
  return payload
}

async function onSubmit(): Promise<void> {
  try {
    await formRef.value?.validate()
  } catch {
    return // 校验失败，ant-design 已在字段下方给出提示
  }

  submitting.value = true
  try {
    if (isEdit.value && props.record) {
      await updateUser(props.record.id, buildPayload())
      message.success('修改成功')
    } else {
      await createUser(buildPayload())
      message.success('新增成功')
    }
    emit('update:open', false)
    emit('saved')
  } catch {
    // 拦截器已提示，保持弹窗打开让用户修正
  } finally {
    submitting.value = false
  }
}

function onCancel(): void {
  emit('update:open', false)
}
</script>
