<template>
  <a-result status="403" title="403" sub-title="抱歉，您没有访问该页面的权限。">
    <template #extra>
      <a-button type="primary" @click="router.push('/')">返回首页</a-button>
    </template>
  </a-result>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()
</script>
