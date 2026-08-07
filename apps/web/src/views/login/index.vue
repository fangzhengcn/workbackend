<template>
  <div class="login-container">
    <KakeyaBackground />

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

    <p class="footnote">
      <span>背景：挂谷猜想 —— 单位针在三尖瓣线内完成 180° 转向</span>
      <!-- 右幕（δ-管束）在 <1024px 时不渲染，故这句同步隐藏，
           否则文案会指着一个看不见的东西 -->
      <span class="footnote-wide">；方向球上的 δ-管束</span>
    </p>
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
import KakeyaBackground from './KakeyaBackground.vue'

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
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  /* 与画布首帧同色：canvas 是 fixed 定位的，这里兜住它之外的区域 */
  background: #1b2540;
}

/* 毛玻璃面板。浅底也仍是深色调，纯白卡片会像补丁，
   半透明 + 背景模糊才能和动画融为一体，同时保证表单文字的对比度。 */
.login-card {
  position: relative;
  z-index: 1;
  width: 380px;
  padding: 8px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(16px) saturate(140%);
  border: 1px solid rgba(255, 255, 255, 0.5);
  box-shadow:
    0 16px 50px rgba(10, 20, 45, 0.45),
    0 0 0 1px rgba(120, 170, 255, 0.16);
  animation: card-in 0.5s ease-out both;
}

/* 卡片入场：轻微上浮。仅一次，不干扰输入 */
@keyframes card-in {
  from {
    opacity: 0;
    transform: translateY(14px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.title {
  margin: 0 0 4px;
  text-align: center;
  font-size: 22px;
  letter-spacing: 1px;
}

.subtitle {
  margin: 0 0 24px;
  text-align: center;
  color: rgba(0, 0, 0, 0.45);
  font-size: 13px;
}

/* 背景的说明文字：压暗到几乎不抢注意力，仅在细看时可见 */
.footnote {
  position: relative;
  z-index: 1;
  margin: 20px 0 0;
  padding: 0 16px;
  text-align: center;
  color: rgba(215, 232, 255, 0.42);
  font-size: 12px;
  letter-spacing: 0.5px;
}

/* 与 KakeyaBackground 的 TWO_SCENE_MIN_WIDTH 对齐：
   两处阈值必须一致，否则文案与画面会不同步 */
.footnote-wide {
  display: none;
}

@media (min-width: 1024px) {
  .footnote-wide {
    display: inline;
  }
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

/* 窄屏：留出左右边距，别让卡片贴边 */
@media (max-width: 420px) {
  .login-card {
    width: calc(100vw - 32px);
  }
}

/* 尊重减弱动效设置：入场动画一并关掉（画布侧已只画静态帧） */
@media (prefers-reduced-motion: reduce) {
  .login-card {
    animation: none;
  }
}
</style>
