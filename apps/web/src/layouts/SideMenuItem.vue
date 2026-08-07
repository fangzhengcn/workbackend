<!--
  递归渲染菜单节点：目录渲染为可展开的子菜单，菜单渲染为可点击项。
  组件内递归引用自身，因此需要显式声明 name（见下方 defineOptions）。
-->
<template>
  <!-- 目录：有可见子节点时渲染为 SubMenu -->
  <a-sub-menu v-if="isDir && visibleChildren.length > 0" :key="menu.path">
    <template #icon><MenuIcon :name="menu.icon" /></template>
    <template #title>{{ menu.name }}</template>
    <SideMenuItem v-for="child in visibleChildren" :key="child.id" :menu="child" :parent-path="fullPath" />
  </a-sub-menu>

  <!-- 菜单：叶子节点，key 为完整路由路径 -->
  <a-menu-item v-else-if="!isDir" :key="fullPath">
    <template #icon><MenuIcon :name="menu.icon" /></template>
    <span>{{ menu.name }}</span>
  </a-menu-item>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { MenuType, Status, type MenuNode } from '@workbackend/shared'
import MenuIcon from '@/components/MenuIcon.vue'

defineOptions({ name: 'SideMenuItem' })

const props = withDefaults(
  defineProps<{
    menu: MenuNode
    /** 父级路径，用于把相对 path 拼成完整路由地址 */
    parentPath?: string
  }>(),
  { parentPath: '' },
)

const isDir = computed(() => props.menu.type === MenuType.Dir)

const visibleChildren = computed(() =>
  (props.menu.children ?? []).filter(
    (child) => child.type !== MenuType.Button && child.visible === Status.Enabled,
  ),
)

/**
 * 拼接完整路径。
 * 后端约定：目录 path 以 / 开头（如 /system），子菜单为相对路径（如 user）。
 */
const fullPath = computed(() => {
  const path = props.menu.path
  if (path.startsWith('/')) return path
  const parent = props.parentPath.endsWith('/')
    ? props.parentPath.slice(0, -1)
    : props.parentPath
  return `${parent}/${path}`
})
</script>
