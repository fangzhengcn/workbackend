<!--
  部门新增/编辑弹窗。
-->
<template>
  <a-modal
    :open="open"
    :title="isEdit ? '编辑部门' : '新增部门'"
    :confirm-loading="submitting"
    width="520px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-form ref="formRef" :model="form" :rules="rules" :label-col="{ span: 5 }">
      <a-form-item label="上级部门" name="parentId">
        <a-tree-select
          v-model:value="form.parentId"
          :tree-data="parentOptions"
          placeholder="不选则为顶级部门"
          allow-clear
          tree-default-expand-all
        />
      </a-form-item>

      <a-form-item label="部门名称" name="name">
        <a-input v-model:value="form.name" placeholder="如：研发部" allow-clear />
      </a-form-item>

      <a-form-item label="负责人" name="leader">
        <a-input v-model:value="form.leader" placeholder="选填" allow-clear />
      </a-form-item>

      <a-form-item label="联系电话" name="phone">
        <a-input v-model:value="form.phone" placeholder="选填" allow-clear />
      </a-form-item>

      <a-form-item label="显示顺序" name="sort">
        <a-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
      </a-form-item>

      <a-form-item label="状态" name="status">
        <a-radio-group v-model:value="form.status">
          <a-radio :value="Status.Enabled">正常</a-radio>
          <a-radio :value="Status.Disabled">停用</a-radio>
        </a-radio-group>
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { message, type FormInstance, type TreeSelectProps } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import { Status, type DeptNode } from '@workbackend/shared'
import { createDept, updateDept, type DeptPayload } from '@/api/system'

const props = defineProps<{
  open: boolean
  record: DeptNode | null
  /** 新增子部门时带入的默认上级 */
  defaultParentId?: number
  /** 完整部门树，用于上级选择 */
  tree: DeptNode[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const submitting = ref(false)

const isEdit = computed(() => props.record !== null)

function emptyForm(): DeptPayload {
  return {
    parentId: undefined,
    name: '',
    leader: '',
    phone: '',
    sort: 0,
    status: Status.Enabled,
  }
}

const form = reactive<DeptPayload>(emptyForm())

/**
 * 上级候选需排除自己与自己的后代。
 *
 * 允许选到后代会让整棵子树脱离根节点、从部门树上消失，
 * 且无法再从界面移回来。后端也有同样校验，前端提前把选项去掉更直接。
 */
const parentOptions = computed<TreeSelectProps['treeData']>(() => {
  const excludeId = props.record?.id
  const convert = (nodes: DeptNode[]): TreeSelectProps['treeData'] =>
    nodes
      .filter((node) => node.id !== excludeId)
      .map((node) => ({
        value: node.id,
        label: node.name,
        children: node.children?.length ? convert(node.children) : undefined,
      }))
  return convert(props.tree)
})

const rules: Record<string, Rule[]> = {
  name: [{ required: true, max: 64, message: '请输入部门名称' }],
  phone: [{ max: 32, message: '联系电话过长' }],
}

watch(
  () => props.open,
  (opened) => {
    if (!opened) return

    Object.assign(form, emptyForm())
    if (props.record) {
      const r = props.record
      Object.assign(form, {
        // 顶级部门 parentId 为 0，转 undefined 以显示占位文案
        parentId: r.parentId === 0 ? undefined : r.parentId,
        name: r.name,
        leader: r.leader,
        phone: r.phone,
        sort: r.sort,
        status: r.status,
      })
    } else if (props.defaultParentId) {
      form.parentId = props.defaultParentId
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

  const payload: DeptPayload = {
    // 未选上级即顶级，后端用 0 表示
    parentId: form.parentId ?? 0,
    name: form.name,
    leader: form.leader,
    phone: form.phone,
    sort: form.sort,
    status: form.status,
  }

  submitting.value = true
  try {
    if (isEdit.value && props.record) {
      await updateDept(props.record.id, payload)
      message.success('修改成功')
    } else {
      await createDept(payload)
      message.success('新增成功')
    }
    emit('update:open', false)
    emit('saved')
  } catch {
    // 拦截器已提示
  } finally {
    submitting.value = false
  }
}

function onCancel(): void {
  emit('update:open', false)
}
</script>
