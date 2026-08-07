<template>
  <div class="login-container">
    <a-card class="login-card" :bordered="false">
      <h2 class="title">权限管理后台</h2>
      <p class="subtitle">请使用您的账号登录</p>

      <a-form :model="form" layout="vertical" @finish="onSubmit">
        <a-form-item
          label="账号"
          name="username"
          :rules="[{ required: true, message: '请输入登录账号' }]"
        >
          <a-input
            v-model:value="form.username"
            size="large"
            placeholder="请输入账号"
            autocomplete="username"
          >
            <template #prefix><UserOutlined /></template>
          </a-input>
        </a-form-item>

        <a-form-item
          label="密码"
          name="password"
          :rules="[{ required: true, message: '请输入密码' }]"
        >
          <a-input-password
            v-model:value="form.password"
            size="large"
            placeholder="请输入密码"
            autocomplete="current-password"
          >
            <template #prefix><LockOutlined /></template>
          </a-input-password>
        </a-form-item>

        <a-form-item
          v-if="captcha"
          label="验证码"
          name="captchaCode"
          :rules="[{ required: true, message: '请输入验证码' }]"
        >
          <div class="captcha-row">
            <a-input
              v-model:value="form.captchaCode"
              size="large"
              placeholder="请输入验证码"
              @press-enter="onSubmit"
            />
            <img
              :src="captcha.imageBase64"
              class="captcha-image"
              alt="点击刷新验证码"
              title="点击刷新"
              @click="loadCaptcha"
            />
          </div>
        </a-form-item>

        <a-alert v-if="errorMessage" type="error" :message="errorMessage" show-icon class="error" />

        <a-form-item>
          <a-button type="primary" size="large" block :loading="loading" html-type="submit">
            登录
          </a-button>
        </a-form-item>
      </a-form>
    </a-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { LockOutlined, UserOutlined } from '@ant-design/icons-vue'
import type { CaptchaResult } from '@workbackend/shared'
import { getCaptcha } from '@/api/auth'
import { usePermissionStore } from '@/store/permission'
import { useUserStore } from '@/store/user'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const permissionStore = usePermissionStore()

const loading = ref(false)
const errorMessage = ref('')
const captcha = ref<CaptchaResult | null>(null)

const form = reactive({
  username: '',
  password: '',
  captchaCode: '',
})

/** 拉取验证码；后端未启用该接口时静默跳过，不阻塞登录 */
async function loadCaptcha() {
  try {
    captcha.value = await getCaptcha()
    form.captchaCode = ''
  } catch {
    captcha.value = null
  }
}

async function onSubmit() {
  if (loading.value) return
  errorMessage.value = ''
  loading.value = true

  // 区分「凭据校验失败」与「登录后初始化失败」：
  // 后者（如拉取菜单出错）若也提示「账号或密码错误」会把人引到完全错误的方向。
  let authenticated = false
  try {
    await userStore.login({
      username: form.username,
      password: form.password,
      captchaId: captcha.value?.captchaId,
      captchaCode: form.captchaCode,
    })
    authenticated = true

    // 登录成功后立刻构建动态路由，避免守卫再跑一轮
    await userStore.fetchInfo()
    const routes = await permissionStore.generateRoutes()
    routes.forEach((item) => router.addRoute(item))

    const redirect = (route.query.redirect as string) || '/'
    await router.replace(redirect)
  } catch (error) {
    const text = authenticated
      ? `登录成功但初始化失败：${extractMessage(error)}`
      : extractMessage(error)
    errorMessage.value = text

    if (authenticated) {
      // 凭据已通过、仅初始化失败时，把登录态清掉再让用户重试，
      // 否则残留的 Token 会让守卫直接放行到一个没有路由的空白页。
      // 这条链路上的接口未设 silent，拦截器已弹过提示，此处不再重复。
      userStore.resetState()
      permissionStore.resetState()
    } else {
      // 登录接口是 silent 的（提示交由页面处理），故这里补一个 toast：
      // 表单中部的错误区块在验证码等长表单下容易被忽略。
      message.error(text)
    }
    // 验证码一次性消费，失败后必须换一张
    if (captcha.value) await loadCaptcha()
  } finally {
    loading.value = false
  }
}

/** 从 Axios 错误中取出后端返回的提示语 */
function extractMessage(error: unknown): string {
  const response = (error as { response?: { data?: { message?: string } } }).response
  if (response?.data?.message) return response.data.message
  if (error instanceof Error && error.message) return error.message
  return '登录失败，请稍后重试'
}

onMounted(loadCaptcha)
</script>

<style scoped>
.login-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #1677ff 0%, #0958d9 100%);
}

.login-card {
  width: 380px;
  padding: 8px;
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
}

.title {
  margin: 0 0 4px;
  text-align: center;
  font-size: 22px;
}

.subtitle {
  margin: 0 0 24px;
  text-align: center;
  color: rgba(0, 0, 0, 0.45);
  font-size: 13px;
}

.captcha-row {
  display: flex;
  gap: 8px;
}

.captcha-image {
  height: 40px;
  width: 120px;
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  cursor: pointer;
  object-fit: cover;
}

.error {
  margin-bottom: 16px;
}
</style>
