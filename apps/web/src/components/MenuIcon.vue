<!--
  按 sys_menu.icon 中的名称渲染 Ant Design 图标。
  数据库存的是字符串（如 "user"、"setting"），需映射到具体图标组件。
  新增图标时在 iconMap 中登记即可。
-->
<template>
  <component :is="icon" v-if="icon" />
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import {
  AppstoreOutlined,
  BookOutlined,
  ClusterOutlined,
  DashboardOutlined,
  FileTextOutlined,
  LoginOutlined,
  MenuOutlined,
  SettingOutlined,
  TeamOutlined,
  UserOutlined,
} from '@ant-design/icons-vue'

const props = defineProps<{ name?: string }>()

const iconMap: Record<string, Component> = {
  setting: SettingOutlined,
  user: UserOutlined,
  team: TeamOutlined,
  menu: MenuOutlined,
  cluster: ClusterOutlined,
  book: BookOutlined,
  'file-text': FileTextOutlined,
  login: LoginOutlined,
  dashboard: DashboardOutlined,
  appstore: AppstoreOutlined,
}

// 未登记的图标名不渲染，避免整块菜单报错
const icon = computed(() => (props.name ? iconMap[props.name] : undefined))
</script>
