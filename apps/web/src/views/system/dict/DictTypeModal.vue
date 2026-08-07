<!--
  字典类型新增/编辑弹窗。
-->
<template>
  <a-modal
    :open="open"
    :title="isEdit ? '编辑字典类型' : '新增字典类型'"
    :confirm-loading="submitting"
    width="500px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-form ref="formRef" :model="form" :rules="rules" :label-col="{ span: 6 }">
      <a-form-item label="字典名称" name="name">
        <a-input v-model:value="form.name" placeholder="如：用户性别" allow-clear />
      </a-form-item>

      <a-form-item label="类型标识" name="type">
        <a-input v-model:value="form.type" placeholder="如：sys_user_gender" allow-clear />
        <!--
          标识是数据项的归属键，也是按类型取数据的 URL 路径段。
          改它会连带更新所有数据项的冗余列（后端在同一事务内完成）。
        -->
        <div v-if="isEdit" class="hint">修改标识会同步更新其下所有数据项</div>
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
import { Status } from '@workbackend/shared'
import {
  createDictType,
  updateDictType,
  type DictTypeItem,
  type DictTypePayload,
} from '@/api/dict'

const props = defineProps<{
  open: boolean
  record: DictTypeItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const submitting = ref(false)

const isEdit = computed(() => props.record !== null)

function emptyForm(): DictTypePayload {
  return { name: '', type: '', status: Status.Enabled, remark: '' }
}

const form = reactive<DictTypePayload>(emptyForm())

const rules: Record<string, Rule[]> = {
  name: [{ required: true, max: 64, message: '请输入字典名称' }],
  type: [
    { required: true, max: 64, message: '请输入类型标识' },
    // 与后端 dictTypePattern 一致，提前拦住而非等接口报错
    {
      pattern: /^[a-zA-Z][a-zA-Z0-9_]*$/,
      message: '只允许字母、数字与下划线，且需以字母开头',
    },
  ],
}

watch(
  () => props.open,
  (opened) => {
    if (!opened) return
    Object.assign(form, emptyForm())
    if (props.record) {
      Object.assign(form, {
        name: props.record.name,
        type: props.record.type,
        status: props.record.status,
        remark: props.record.remark,
      })
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

  submitting.value = true
  try {
    if (isEdit.value && props.record) {
      await updateDictType(props.record.id, { ...form })
      message.success('修改成功')
    } else {
      await createDictType({ ...form })
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

<style scoped>
.hint {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
