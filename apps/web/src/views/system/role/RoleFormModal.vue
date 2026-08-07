<!--
  角色新增/编辑弹窗。
-->
<template>
  <a-modal
    :open="open"
    :title="isEdit ? '编辑角色' : '新增角色'"
    :confirm-loading="submitting"
    width="560px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-form ref="formRef" :model="form" :rules="rules" :label-col="{ span: 5 }">
      <a-form-item label="角色名称" name="name">
        <a-input v-model:value="form.name" placeholder="如：运营人员" allow-clear />
      </a-form-item>

      <a-form-item label="角色标识" name="code">
        <!--
          标识是 Casbin 策略与 JWT 中的角色主键，创建后不可改：
          已签发的 Token 里带着旧标识，改了会让这些 Token 的权限判定错位。
        -->
        <a-input
          v-model:value="form.code"
          :disabled="isEdit"
          placeholder="字母/数字，如 operator"
          allow-clear
        />
        <div v-if="isEdit" class="hint">标识创建后不可修改</div>
      </a-form-item>

      <a-form-item label="显示顺序" name="sort">
        <a-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
      </a-form-item>

      <a-form-item label="数据范围" name="dataScope">
        <a-select v-model:value="form.dataScope" :options="dataScopeOptions" />
      </a-form-item>

      <a-form-item v-if="form.dataScope === DataScope.Custom" label="可见部门" name="deptIds">
        <a-tree-select
          v-model:value="form.deptIds"
          :tree-data="deptTree"
          :field-names="{ label: 'name', value: 'id', children: 'children' }"
          tree-checkable
          multiple
          placeholder="自定义范围需至少选择一个部门"
          tree-default-expand-all
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
import { DataScope, Status, type DeptNode, type RoleItem } from '@workbackend/shared'
import { createRole, getRole, updateRole, type RolePayload } from '@/api/role'
import { getDeptTree } from '@/api/system'

const props = defineProps<{
  open: boolean
  record: RoleItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const submitting = ref(false)
const deptTree = ref<DeptNode[]>([])

const isEdit = computed(() => props.record !== null)

const dataScopeOptions = [
  { label: '全部数据', value: DataScope.All },
  { label: '自定义', value: DataScope.Custom },
  { label: '本部门', value: DataScope.Dept },
  { label: '本部门及子部门', value: DataScope.DeptTree },
  { label: '仅本人', value: DataScope.Self },
]

function emptyForm(): RolePayload {
  return {
    name: '',
    code: '',
    sort: 0,
    dataScope: DataScope.Dept,
    status: Status.Enabled,
    remark: '',
    deptIds: [],
  }
}

const form = reactive<RolePayload>(emptyForm())

const rules = computed<Record<string, Rule[]>>(() => {
  const base: Record<string, Rule[]> = {
    name: [{ required: true, max: 64, message: '请输入角色名称' }],
  }
  if (!isEdit.value) {
    base.code = [
      { required: true, max: 64, message: '请输入角色标识' },
      // 后端 binding 用 alphanumunicode 校验，含 - 或空格会被拒；
      // 这里提前拦住，免得用户提交后才看到一句笼统的参数错误。
      { pattern: /^[a-zA-Z0-9_]+$/, message: '只允许字母、数字与下划线' },
    ]
  }
  return base
})

/**
 * 每次打开都重新拉部门树。
 *
 * 不缓存的原因同 UserFormModal：部门在别的页面维护，
 * 缓存会让新建的部门在本弹窗里永远不出现。
 */
async function loadDeptTree(): Promise<void> {
  try {
    deptTree.value = (await getDeptTree()) ?? []
  } catch {
    // 拦截器已提示
  }
}

watch(
  () => props.open,
  async (opened) => {
    if (!opened) return
    await loadDeptTree()

    Object.assign(form, emptyForm())
    if (props.record) {
      const r = props.record
      Object.assign(form, {
        name: r.name,
        code: r.code,
        sort: r.sort,
        dataScope: r.dataScope,
        status: r.status,
        remark: r.remark,
      })
      // 自定义数据范围要回显已选部门，需额外取详情。
      if (r.dataScope === DataScope.Custom) {
        try {
          const detail = await getRole(r.id)
          form.deptIds = detail.deptIds ?? []
        } catch {
          // 拦截器已提示
        }
      }
    }
    formRef.value?.clearValidate()
  },
)

async function onSubmit(): Promise<void> {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  // 与后端同一条规则，前端先拦一道给出明确指引。
  if (form.dataScope === DataScope.Custom && (form.deptIds?.length ?? 0) === 0) {
    message.warning('自定义数据范围需至少选择一个部门')
    return
  }

  const payload: RolePayload = {
    name: form.name,
    sort: form.sort,
    dataScope: form.dataScope,
    status: form.status,
    remark: form.remark,
  }
  // 部门只在自定义范围下才有意义，其余范围传了也会被后端清空。
  if (form.dataScope === DataScope.Custom) {
    payload.deptIds = form.deptIds
  }

  submitting.value = true
  try {
    if (isEdit.value && props.record) {
      await updateRole(props.record.id, payload)
      message.success('修改成功')
    } else {
      await createRole({ ...payload, code: form.code })
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

<style scoped>
.hint {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
