<!--
  重置密码弹窗。

  单独成一个组件而不塞进编辑弹窗：重置密码是独立权限点
  （system:user:resetPwd）与独立接口，混在编辑表单里会让「有编辑权但无重置权」
  的角色也看到密码框。
-->
<template>
  <a-modal
    :open="open"
    title="重置密码"
    :confirm-loading="submitting"
    width="440px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-alert
      v-if="record"
      :message="`即将重置账号「${record.username}」的密码`"
      type="warning"
      show-icon
      style="margin-bottom: 16px"
    />
    <a-form ref="formRef" :model="form" :rules="rules" :label-col="{ span: 6 }">
      <a-form-item label="新密码" name="password">
        <a-input-password v-model:value="form.password" placeholder="6-64 位" />
      </a-form-item>
      <a-form-item label="确认密码" name="confirm">
        <a-input-password v-model:value="form.confirm" placeholder="再输入一次" />
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import type { UserItem } from '@workbackend/shared'
import { resetPassword } from '@/api/user'

const props = defineProps<{
  open: boolean
  record: UserItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const submitting = ref(false)

const form = reactive({ password: '', confirm: '' })

const rules: Record<string, Rule[]> = {
  password: [{ required: true, min: 6, max: 64, message: '密码需 6-64 位' }],
  confirm: [
    { required: true, message: '请再次输入密码' },
    {
      // 两次输入一致性只能用自定义校验器，async-validator 无内置规则
      validator: (_rule: Rule, value: string) =>
        value === form.password ? Promise.resolve() : Promise.reject('两次输入的密码不一致'),
    },
  ],
}

watch(
  () => props.open,
  (opened) => {
    if (!opened) return
    form.password = ''
    form.confirm = ''
    formRef.value?.clearValidate()
  },
)

async function onSubmit(): Promise<void> {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  if (!props.record) return

  submitting.value = true
  try {
    await resetPassword(props.record.id, form.password)
    message.success('密码重置成功')
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
