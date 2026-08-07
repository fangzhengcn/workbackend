<!--
  角色菜单授权弹窗。
-->
<template>
  <a-modal
    :open="open"
    title="分配菜单权限"
    :confirm-loading="submitting"
    width="520px"
    @ok="onSubmit"
    @cancel="onCancel"
  >
    <div v-if="record" class="role-name">角色：{{ record.name }}（{{ record.code }}）</div>

    <a-space class="tools">
      <a-button size="small" @click="expandAll(true)">展开全部</a-button>
      <a-button size="small" @click="expandAll(false)">收起全部</a-button>
      <a-button size="small" @click="checkAll(true)">全选</a-button>
      <a-button size="small" @click="checkAll(false)">全不选</a-button>
    </a-space>

    <a-spin :spinning="loading">
      <a-tree
        v-model:checked-keys="checkedKeys"
        v-model:expanded-keys="expandedKeys"
        :tree-data="treeData"
        checkable
        check-strictly
        class="tree"
      />
      <a-empty v-if="!loading && !treeData?.length" description="暂无菜单" />
    </a-spin>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import type { TreeProps } from 'ant-design-vue'
import type { MenuNode, RoleItem } from '@workbackend/shared'
import { assignMenus, getRoleMenuIds } from '@/api/role'
import { getMenuTree } from '@/api/system'

const props = defineProps<{
  open: boolean
  record: RoleItem | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  saved: []
}>()

const loading = ref(false)
const submitting = ref(false)
/*
 * 转成 a-tree 要求的 { key, title, children } 结构，而不是用 field-names 映射。
 * field-names 只在运行期改读取的字段名，类型上仍要求 DataNode 自带 key，
 * 直接把 MenuNode[] 传进去过不了类型检查。显式映射同时也让「按钮节点也要
 * 可勾选」这件事更清楚——按钮没有 path/component，但仍是权限点。
 */
const treeData = ref<TreeProps['treeData']>([])
const expandedKeys = ref<number[]>([])
/** 全部节点 ID，供全选与展开全部使用 */
const allKeys = ref<number[]>([])

/*
 * check-strictly 模式下 a-tree 的 v-model:checked-keys 是
 * { checked: Key[]; halfChecked: Key[] } 而非裸数组。
 *
 * 为什么必须用 check-strictly：默认的联动模式下，父节点会因「子节点未全选」
 * 而只进入 halfChecked，不出现在 checked 里。提交时父目录就被漏掉，
 * 于是「系统管理」这个目录的授权在保存后丢失，菜单直接从侧边栏消失。
 * 父子各自独立勾选才能如实表达 sys_role_menu 里的行。
 */
const checkedKeys = ref<{ checked: number[]; halfChecked: number[] }>({
  checked: [],
  halfChecked: [],
})

/** 把菜单树转成 a-tree 节点，同时收集全部 ID */
function toTreeData(nodes: MenuNode[], collected: number[]): TreeProps['treeData'] {
  return nodes.map((node) => {
    collected.push(node.id)
    return {
      key: node.id,
      title: node.name,
      children: node.children?.length ? toTreeData(node.children, collected) : undefined,
    }
  })
}

function expandAll(open: boolean): void {
  expandedKeys.value = open ? [...allKeys.value] : []
}

function checkAll(all: boolean): void {
  checkedKeys.value = { checked: all ? [...allKeys.value] : [], halfChecked: [] }
}

watch(
  () => props.open,
  async (opened) => {
    if (!opened || !props.record) return

    loading.value = true
    try {
      // 菜单树与已勾选项每次打开都重新取。
      // 菜单树不缓存：菜单在菜单管理页维护，缓存会让新建的菜单在授权树里
      // 永远不出现，而「新增菜单后立刻给角色授权」正是最常见的操作顺序。
      // 已勾选项更必须每次取，否则切换角色会看到上一个角色的勾选状态。
      const [menus, assigned] = await Promise.all([
        getMenuTree(),
        getRoleMenuIds(props.record.id),
      ])

      const collected: number[] = []
      treeData.value = toTreeData(menus ?? [], collected)
      allKeys.value = collected
      expandedKeys.value = [...collected]
      checkedKeys.value = { checked: assigned ?? [], halfChecked: [] }
    } catch {
      // 拦截器已提示
    } finally {
      loading.value = false
    }
  },
)

async function onSubmit(): Promise<void> {
  if (!props.record) return

  submitting.value = true
  try {
    await assignMenus(props.record.id, checkedKeys.value.checked)
    message.success('分配成功，权限已即时生效')
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
.role-name {
  margin-bottom: 8px;
  color: rgba(0, 0, 0, 0.65);
}

.tools {
  margin-bottom: 8px;
}

.tree {
  max-height: 420px;
  overflow: auto;
}
</style>
