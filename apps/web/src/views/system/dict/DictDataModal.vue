<!--
  字典数据项新增/编辑弹窗。
-->
<template>
  <a-modal
    :open="open"
    :title="isEdit ? '编辑字典数据' : '新增字典数据'"
    :confirm-loading="submitting"
    width="500px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-form ref="formRef" :model="form" :rules="rules" :label-col="{ span: 6 }">
      <a-form-item label="所属类型">
        <!-- 归属类型不可改：改归属等于把数据搬到另一个字典下，语义上应删了重建 -->
        <a-input :value="dictTypeName" disabled />
      </a-form-item>

      <a-form-item label="字典标签" name="label">
        <a-input v-model:value="form.label" placeholder="展示给用户的文字，如：男" allow-clear />
      </a-form-item>

      <a-form-item label="字典键值" name="value">
        <a-input v-model:value="form.value" placeholder="程序中使用的值，如：1" allow-clear />
      </a-form-item>

      <a-form-item label="显示顺序" name="sort">
        <a-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
      </a-form-item>

      <a-form-item label="是否默认" name="isDefault">
        <a-radio-group v-model:value="form.isDefault">
          <a-radio :value="1">是</a-radio>
          <a-radio :value="0">否</a-radio>
        </a-radio-group>
        <!-- 每个类型最多一个默认项，后端会自动清掉原有的默认标记 -->
        <div v-if="form.isDefault === 1" class="hint">设为默认会取消该类型下原有的默认项</div>
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
  createDictData,
  updateDictData,
  type DictDataItem,
  type DictDataPayload,
  type DictTypeItem,
} from '@/api/dict'

const props = defineProps<{
  open: boolean
  record: DictDataItem | null
  /** 当前选中的字典类型，新增时决定归属 */
  dictType: DictTypeItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const submitting = ref(false)

const isEdit = computed(() => props.record !== null)

const dictTypeName = computed(() =>
  props.dictType ? `${props.dictType.name}（${props.dictType.type}）` : '',
)

function emptyForm(): DictDataPayload {
  return {
    label: '',
    value: '',
    sort: 0,
    isDefault: 0,
    status: Status.Enabled,
    remark: '',
  }
}

const form = reactive<DictDataPayload>(emptyForm())

const rules: Record<string, Rule[]> = {
  label: [{ required: true, max: 100, message: '请输入字典标签' }],
  value: [{ required: true, max: 100, message: '请输入字典键值' }],
}

watch(
  () => props.open,
  (opened) => {
    if (!opened) return
    Object.assign(form, emptyForm())
    if (props.record) {
      Object.assign(form, {
        label: props.record.label,
        value: props.record.value,
        sort: props.record.sort,
        isDefault: props.record.isDefault,
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
      await updateDictData(props.record.id, { ...form })
      message.success('修改成功')
    } else {
      if (!props.dictType) {
        message.warning('请先选择字典类型')
        return
      }
      // dictTypeId 只在新增时传；后端据此推导冗余的 dictType 列，
      // 不由前端传字符串，避免两者不一致。
      await createDictData({ ...form, dictTypeId: props.dictType.id })
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
