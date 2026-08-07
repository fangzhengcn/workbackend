<!--
  基础布局：左侧菜单 + 顶栏 + 内容区。
  菜单项由 permission store 的菜单树渲染，只展示 visible=1 的非按钮节点。
-->
<template>
  <a-layout class="basic-layout">
    <a-layout-sider v-model:collapsed="collapsed" collapsible theme="dark" :width="220">
      <div class="logo">
        <span v-if="!collapsed">权限管理后台</span>
        <span v-else>RBAC</span>
      </div>
      <a-menu
        v-model:selectedKeys="selectedKeys"
        v-model:openKeys="openKeys"
        theme="dark"
        mode="inline"
        @click="onMenuClick"
      >
        <a-menu-item key="/dashboard">
          <template #icon><DashboardOutlined /></template>
          <span>首页</span>
        </a-menu-item>
        <SideMenuItem v-for="menu in visibleMenus" :key="menu.id" :menu="menu" />
      </a-menu>
    </a-layout-sider>

    <a-layout>
      <a-layout-header class="header">
        <a-button type="text" @click="collapsed = !collapsed">
          <MenuUnfoldOutlined v-if="collapsed" />
          <MenuFoldOutlined v-else />
        </a-button>

        <a-dropdown>
          <a class="user-info" @click.prevent>
            <a-avatar :src="userStore.info?.avatar || undefined" size="small">
              {{ displayName.charAt(0) }}
            </a-avatar>
            <span class="username">{{ displayName }}</span>
          </a>
          <template #overlay>
            <a-menu>
              <a-menu-item key="profile" @click="router.push('/profile')">
                <UserOutlined /> 个人中心
              </a-menu-item>
              <a-menu-divider />
              <a-menu-item key="logout" @click="onLogout">
                <LogoutOutlined /> 退出登录
              </a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
      </a-layout-header>

      <a-layout-content class="content">
        <router-view v-slot="{ Component }">
          <keep-alive>
            <component :is="Component" />
          </keep-alive>
        </router-view>
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Modal } from 'ant-design-vue'
import {
  DashboardOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  UserOutlined,
} from '@ant-design/icons-vue'
import { MenuType, Status } from '@workbackend/shared'
import { usePermissionStore } from '@/store/permission'
import { useUserStore } from '@/store/user'
import SideMenuItem from './SideMenuItem.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const permissionStore = usePermissionStore()

const collapsed = ref(false)
const selectedKeys = ref<string[]>([route.path])
const openKeys = ref<string[]>([])

// 按钮节点与隐藏节点不进侧边栏
const visibleMenus = computed(() =>
  permissionStore.menus.filter(
    (menu) => menu.type !== MenuType.Button && menu.visible === Status.Enabled,
  ),
)

const displayName = computed(
  () => userStore.info?.nickname || userStore.info?.username || '未登录',
)

// 路由变化时同步菜单高亮，处理直接输 URL 或前进后退的情况
watch(
  () => route.path,
  (path) => {
    selectedKeys.value = [path]
  },
  { immediate: true },
)

function onMenuClick({ key }: { key: string | number }) {
  const path = String(key)
  if (path !== route.path) {
    router.push(path)
  }
}

function onLogout() {
  Modal.confirm({
    title: '确认退出登录？',
    content: '退出后需要重新登录才能继续操作。',
    okText: '确认退出',
    cancelText: '取消',
    async onOk() {
      await userStore.logout()
      permissionStore.resetState()
      // 用 location 跳转而非 router.push：强制刷新以清空已注册的动态路由
      window.location.href = '/login'
    },
  })
}
</script>

<style scoped>
.basic-layout {
  min-height: 100vh;
}

.logo {
  height: 48px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 16px;
  font-weight: 600;
  background: rgba(255, 255, 255, 0.08);
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  background: #fff;
  box-shadow: 0 1px 4px rgba(0, 21, 41, 0.08);
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  color: rgba(0, 0, 0, 0.85);
}

.content {
  margin: 16px;
  padding: 16px;
  background: #fff;
  border-radius: 6px;
  min-height: calc(100vh - 112px);
}
</style>
