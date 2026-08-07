<!--
  菜单新增/编辑弹窗。

  字段按类型（目录/菜单/按钮）动态显隐：按钮不需要路由与组件，
  目录不需要组件。把无关字段直接藏掉，比留着让用户猜要填什么更清楚。
-->
<template>
  <a-modal
    :open="open"
    :title="isEdit ? '编辑菜单' : '新增菜单'"
    :confirm-loading="submitting"
    width="600px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <a-form ref="formRef" :model="form" :rules="rules" :label-col="{ span: 5 }">
      <a-form-item label="上级菜单" name="parentId">
        <a-tree-select
          v-model:value="form.parentId"
          :tree-data="parentOptions"
          placeholder="不选则为顶级菜单"
          allow-clear
          tree-default-expand-all
        />
      </a-form-item>

      <a-form-item label="菜单类型" name="type">
        <a-radio-group v-model:value="form.type">
          <a-radio :value="MenuType.Dir">目录</a-radio>
          <a-radio :value="MenuType.Menu">菜单</a-radio>
          <a-radio :value="MenuType.Button">按钮</a-radio>
        </a-radio-group>
      </a-form-item>

      <a-form-item label="名称" name="name">
        <a-input v-model:value="form.name" placeholder="如：用户管理" allow-clear />
      </a-form-item>

      <a-form-item v-if="!isButton" label="路由地址" name="path">
        <a-input
          v-model:value="form.path"
          :placeholder="form.type === MenuType.Dir ? '如 /system' : '如 user'"
          allow-clear
        />
      </a-form-item>

      <a-form-item v-if="form.type === MenuType.Menu" label="组件路径" name="component">
        <a-input v-model:value="form.component" placeholder="如 system/user/index" allow-clear />
        <!--
          这个路径必须对应 views/ 下真实存在的 .vue 文件：
          前端用 import.meta.glob 静态映射，写错会退化成 404 页且只在控制台报错。
        -->
        <div class="hint">须对应 views/ 下真实存在的文件，否则页面显示 404</div>
      </a-form-item>

      <a-form-item v-if="!isDir" label="权限标识" name="perms">
        <a-input
          v-model:value="form.perms"
          placeholder="如 system:user:add"
          allow-clear
        />
        <div class="hint">格式「模块:资源:操作」，需与后端保持一致</div>
      </a-form-item>

      <a-form-item v-if="!isButton" label="图标" name="icon">
        <a-input v-model:value="form.icon" placeholder="如 user、setting" allow-clear />
      </a-form-item>

      <a-form-item label="显示顺序" name="sort">
        <a-input-number v-model:value="form.sort" :min="0" style="width: 100%" />
      </a-form-item>

      <a-form-item v-if="!isButton" label="侧边栏显示" name="visible">
        <a-radio-group v-model:value="form.visible">
          <a-radio :value="Status.Enabled">显示</a-radio>
          <a-radio :value="Status.Disabled">隐藏</a-radio>
        </a-radio-group>
      </a-form-item>

      <a-form-item label="状态" name="status">
        <a-radio-group v-model:value="form.status">
          <a-radio :value="Status.Enabled">正常</a-radio>
          <a-radio :value="Status.Disabled">停用</a-radio>
        </a-radio-group>
        <div v-if="!isButton" class="hint">停用后该菜单不生成路由，直接访问 URL 也进不去</div>
      </a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { message, type FormInstance, type TreeSelectProps } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import { MenuType, Status, type MenuNode } from '@workbackend/shared'
import { createMenu, updateMenu, type MenuPayload } from '@/api/system'

const props = defineProps<{
  open: boolean
  /** 传入则为编辑态 */
  record: MenuNode | null
  /** 新增时的默认上级（点某行的「新增子菜单」时带入） */
  defaultParentId?: number
  /** 完整菜单树，用于上级菜单选择 */
  tree: MenuNode[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const formRef = ref<FormInstance>()
const submitting = ref(false)

const isEdit = computed(() => props.record !== null)
const isButton = computed(() => form.type === MenuType.Button)
const isDir = computed(() => form.type === MenuType.Dir)

function emptyForm(): MenuPayload {
  return {
    parentId: undefined,
    name: '',
    type: MenuType.Menu,
    path: '',
    component: '',
    perms: '',
    icon: '',
    sort: 0,
    visible: Status.Enabled,
    status: Status.Enabled,
    isFrame: 0,
  }
}

const form = reactive<MenuPayload>(emptyForm())

/**
 * 上级菜单候选：排除按钮（不能作容器），编辑时还要排除自己与自己的后代
 * （否则会把子树挂到自己下面，整棵子树从菜单树消失且无法从界面移回）。
 */
const parentOptions = computed<TreeSelectProps['treeData']>(() => {
  const excludeId = props.record?.id
  const convert = (nodes: MenuNode[]): TreeSelectProps['treeData'] =>
    nodes
      .filter((node) => node.type !== MenuType.Button && node.id !== excludeId)
      .map((node) => ({
        value: node.id,
        label: node.name,
        children: node.children?.length ? convert(node.children) : undefined,
      }))
  return convert(props.tree)
})

const rules = computed<Record<string, Rule[]>>(() => {
  const base: Record<string, Rule[]> = {
    name: [{ required: true, max: 64, message: '请输入菜单名称' }],
    type: [{ required: true, message: '请选择菜单类型' }],
  }
  // 与后端 validateShape 保持一致，提前给出明确指引
  if (form.type === MenuType.Dir || form.type === MenuType.Menu) {
    base.path = [{ required: true, max: 200, message: '请输入路由地址' }]
  }
  if (form.type === MenuType.Menu) {
    base.component = [{ required: true, max: 255, message: '请输入组件路径' }]
  }
  if (form.type === MenuType.Button) {
    base.perms = [{ required: true, max: 100, message: '按钮必须填写权限标识' }]
  }
  return base
})

watch(
  () => props.open,
  (opened) => {
    if (!opened) return

    Object.assign(form, emptyForm())
    if (props.record) {
      const r = props.record
      Object.assign(form, {
        // 顶级菜单的 parentId 是 0，转成 undefined 让选择框显示占位文案
        parentId: r.parentId === 0 ? undefined : r.parentId,
        name: r.name,
        type: r.type,
        path: r.path,
        component: r.component,
        perms: r.perms,
        icon: r.icon,
        sort: r.sort,
        visible: r.visible,
        status: r.status,
        isFrame: r.isFrame,
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

  const payload: MenuPayload = {
    // 未选上级即顶级，后端用 0 表示
    parentId: form.parentId ?? 0,
    name: form.name,
    type: form.type,
    sort: form.sort,
    status: form.status,
    isFrame: form.isFrame,
  }
  // 只提交与当前类型相关的字段，避免把上次填的残留值带进去
  if (!isButton.value) {
    payload.path = form.path
    payload.icon = form.icon
    payload.visible = form.visible
  }
  if (form.type === MenuType.Menu) {
    payload.component = form.component
  }
  if (!isDir.value) {
    payload.perms = form.perms
  }

  submitting.value = true
  try {
    if (isEdit.value && props.record) {
      await updateMenu(props.record.id, payload)
      message.success('修改成功')
    } else {
      await createMenu(payload)
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
  line-height: 1.6;
}
</style>
