<template>
  <div>
    <a-descriptions title="当前登录用户" bordered :column="2">
      <a-descriptions-item label="账号">{{ info?.username }}</a-descriptions-item>
      <a-descriptions-item label="昵称">{{ info?.nickname || '—' }}</a-descriptions-item>
      <a-descriptions-item label="手机号">{{ info?.phone || '—' }}</a-descriptions-item>
      <a-descriptions-item label="邮箱">{{ info?.email || '—' }}</a-descriptions-item>
      <a-descriptions-item label="部门">{{ info?.deptName || '—' }}</a-descriptions-item>
      <a-descriptions-item label="最后登录">{{ info?.lastLoginAt || '—' }}</a-descriptions-item>
      <a-descriptions-item label="角色" :span="2">
        <a-tag v-for="role in info?.roles" :key="role" color="blue">{{ role }}</a-tag>
      </a-descriptions-item>
    </a-descriptions>

    <a-alert
      class="tip"
      type="info"
      show-icon
      message="手机号与邮箱在后端以 AES-256-GCM 密文存储，此处展示的是脱敏结果。"
    />

    <a-card title="权限点" size="small" class="perms">
      <template v-if="userStore.isSuperAdmin">
        <a-tag color="red">超级管理员：拥有全部权限</a-tag>
      </template>
      <template v-else>
        <a-tag v-for="perm in perms" :key="perm">{{ perm }}</a-tag>
        <a-empty v-if="perms.length === 0" description="暂无权限点" />
      </template>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const info = computed(() => userStore.info)
const perms = computed(() => Array.from(userStore.perms))
</script>

<style scoped>
.tip,
.perms {
  margin-top: 16px;
}
</style>
