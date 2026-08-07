<!--
  个人中心：查看并修改自己的资料、修改密码。

  与用户管理的区别：这里只能改自己，且开放的字段不含部门/角色/状态
  ——那些属于管理员职权，放进本页等于让任何人给自己提权。
-->
<template>
  <a-row :gutter="16">
    <!-- 左：只读的账号概览 -->
    <a-col :span="8">
      <a-card size="small" title="账号信息">
        <div class="avatar-block">
          <a-avatar :src="info?.avatar || undefined" :size="88">
            <template #icon><UserOutlined /></template>
          </a-avatar>
          <a-upload
            :show-upload-list="false"
            :before-upload="onBeforeUpload"
            accept="image/jpeg,image/png,image/gif,image/webp"
          >
            <a-button size="small" :loading="uploadingAvatar" class="avatar-btn">
              <UploadOutlined /> 更换头像
            </a-button>
          </a-upload>
          <div class="hint">支持 JPG/PNG/GIF/WEBP，不超过 2MB</div>
        </div>

        <a-descriptions :column="1" size="small">
          <a-descriptions-item label="登录账号">
            {{ info?.username ?? '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="所属部门">
            {{ info?.deptName || '—' }}
          </a-descriptions-item>
          <a-descriptions-item label="拥有角色">
            <a-tag v-for="role in info?.roles ?? []" :key="role" color="blue">{{ role }}</a-tag>
            <span v-if="(info?.roles ?? []).length === 0" class="muted">未分配</span>
          </a-descriptions-item>
          <a-descriptions-item label="最后登录">
            {{ info?.lastLoginAt ?? '—' }}
          </a-descriptions-item>
        </a-descriptions>
        <a-alert
          message="账号、部门与角色需由管理员调整"
          type="info"
          show-icon
          style="margin-top: 8px"
        />
      </a-card>
    </a-col>

    <!-- 右：可编辑的资料与密码 -->
    <a-col :span="16">
      <a-card size="small">
        <a-tabs v-model:activeKey="activeTab">
          <a-tab-pane key="profile" tab="基本资料">
            <a-form
              ref="profileFormRef"
              :model="profileForm"
              :rules="profileRules"
              :label-col="{ span: 5 }"
              style="max-width: 520px"
            >
              <a-form-item label="昵称" name="nickname">
                <a-input v-model:value="profileForm.nickname" allow-clear />
              </a-form-item>

              <a-form-item label="手机号" name="phone">
                <!--
                  展示的是脱敏值，故留空表示不修改：
                  把 138****8000 原样提交会被当明文加密入库，真实号码就毁了。
                -->
                <a-input v-model:value="profileForm.phone" placeholder="留空表示不修改" allow-clear />
                <div class="hint">当前：{{ info?.phone || '未填写' }}</div>
              </a-form-item>

              <a-form-item label="邮箱" name="email">
                <a-input v-model:value="profileForm.email" placeholder="留空表示不修改" allow-clear />
                <div class="hint">当前：{{ info?.email || '未填写' }}</div>
              </a-form-item>

              <a-form-item label="性别" name="gender">
                <a-radio-group v-model:value="profileForm.gender">
                  <a-radio :value="Gender.Unknown">未知</a-radio>
                  <a-radio :value="Gender.Male">男</a-radio>
                  <a-radio :value="Gender.Female">女</a-radio>
                </a-radio-group>
              </a-form-item>

              <a-form-item :wrapper-col="{ offset: 5 }">
                <a-button type="primary" :loading="savingProfile" @click="onSaveProfile">
                  保存
                </a-button>
              </a-form-item>
            </a-form>
          </a-tab-pane>

          <a-tab-pane key="password" tab="修改密码">
            <a-form
              ref="pwdFormRef"
              :model="pwdForm"
              :rules="pwdRules"
              :label-col="{ span: 5 }"
              style="max-width: 520px"
            >
              <a-form-item label="当前密码" name="oldPassword">
                <a-input-password v-model:value="pwdForm.oldPassword" />
              </a-form-item>
              <a-form-item label="新密码" name="newPassword">
                <a-input-password v-model:value="pwdForm.newPassword" placeholder="6-64 位" />
              </a-form-item>
              <a-form-item label="确认新密码" name="confirm">
                <a-input-password v-model:value="pwdForm.confirm" />
              </a-form-item>
              <a-form-item :wrapper-col="{ offset: 5 }">
                <a-button type="primary" :loading="savingPwd" @click="onChangePassword">
                  修改密码
                </a-button>
              </a-form-item>
            </a-form>
          </a-tab-pane>
        </a-tabs>
      </a-card>
    </a-col>
  </a-row>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message, type FormInstance } from 'ant-design-vue'
import type { Rule } from 'ant-design-vue/es/form'
import { UploadOutlined, UserOutlined } from '@ant-design/icons-vue'
import { Gender } from '@workbackend/shared'
import { changePassword, updateProfile, uploadAvatar, type ProfilePayload } from '@/api/auth'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()
const info = computed(() => userStore.info)

const activeTab = ref('profile')

// ---- 头像 ----
const uploadingAvatar = ref(false)

/** 单张头像上限 2MB，与后端 MaxAvatarSize 保持一致 */
const MAX_AVATAR_SIZE = 2 * 1024 * 1024

/**
 * 用 before-upload 接管上传：a-upload 默认会自己发请求，
 * 但那条链路不走我们的 axios 实例，拿不到 Authorization 头。
 * 返回 false 阻止其默认行为，由自己调 API。
 */
function onBeforeUpload(file: File): boolean {
  if (file.size > MAX_AVATAR_SIZE) {
    // 前端先挡一道，省掉一次注定失败的上传往返
    message.error('图片不能超过 2MB')
    return false
  }
  void doUploadAvatar(file)
  return false
}

async function doUploadAvatar(file: File): Promise<void> {
  uploadingAvatar.value = true
  try {
    await uploadAvatar(file)
    // 重新拉取用户信息，让本页与顶栏的头像同时刷新
    await userStore.fetchInfo()
    message.success('头像已更新')
  } catch {
    // 拦截器已提示（如「只支持 JPG/PNG/GIF/WEBP 图片」）
  } finally {
    uploadingAvatar.value = false
  }
}

// ---- 基本资料 ----
const profileFormRef = ref<FormInstance>()
const savingProfile = ref(false)

const profileForm = reactive<ProfilePayload>({
  nickname: '',
  phone: '',
  email: '',
  gender: Gender.Unknown,
})

const profileRules: Record<string, Rule[]> = {
  nickname: [{ max: 64, message: '昵称过长' }],
  // 选填，但填了必须合法——留空时 async-validator 会跳过规则
  phone: [{ pattern: /^1\d{10}$/, message: '请输入 11 位手机号' }],
  email: [{ type: 'email', message: '邮箱格式不正确' }],
}

function resetProfileForm(): void {
  profileForm.nickname = info.value?.nickname ?? ''
  profileForm.gender = info.value?.gender ?? Gender.Unknown
  // 手机号与邮箱不回填，理由见模板注释
  profileForm.phone = ''
  profileForm.email = ''
}

async function onSaveProfile(): Promise<void> {
  try {
    await profileFormRef.value?.validate()
  } catch {
    return
  }

  // 空值代表「不修改」，必须剔除而非发空串（空串会被当成显式清空）
  const payload: ProfilePayload = {
    nickname: profileForm.nickname,
    gender: profileForm.gender,
  }
  if (profileForm.phone) payload.phone = profileForm.phone
  if (profileForm.email) payload.email = profileForm.email

  savingProfile.value = true
  try {
    await updateProfile(payload)
    // 重新拉取用户信息，让顶栏昵称与本页展示同步更新
    await userStore.fetchInfo()
    resetProfileForm()
    message.success('保存成功')
  } catch {
    // 拦截器已提示
  } finally {
    savingProfile.value = false
  }
}

// ---- 修改密码 ----
const pwdFormRef = ref<FormInstance>()
const savingPwd = ref(false)

const pwdForm = reactive({ oldPassword: '', newPassword: '', confirm: '' })

const pwdRules: Record<string, Rule[]> = {
  oldPassword: [{ required: true, message: '请输入当前密码' }],
  newPassword: [
    { required: true, min: 6, max: 64, message: '新密码需 6-64 位' },
    {
      // 后端用 nefield=OldPassword 校验，前端提前拦住给出更明确的提示
      validator: (_rule: Rule, value: string) =>
        value && value === pwdForm.oldPassword
          ? Promise.reject('新密码不能与当前密码相同')
          : Promise.resolve(),
    },
  ],
  confirm: [
    { required: true, message: '请再次输入新密码' },
    {
      validator: (_rule: Rule, value: string) =>
        value === pwdForm.newPassword ? Promise.resolve() : Promise.reject('两次输入的密码不一致'),
    },
  ],
}

async function onChangePassword(): Promise<void> {
  try {
    await pwdFormRef.value?.validate()
  } catch {
    return
  }

  savingPwd.value = true
  try {
    await changePassword(pwdForm.oldPassword, pwdForm.newPassword)
    message.success('密码修改成功，请使用新密码重新登录')
    pwdForm.oldPassword = ''
    pwdForm.newPassword = ''
    pwdForm.confirm = ''
    pwdFormRef.value?.clearValidate()
  } catch {
    // 拦截器已提示（如「原密码错误」）
  } finally {
    savingPwd.value = false
  }
}

onMounted(async () => {
  // 直接进本页时（如刷新）store 里可能还没有 info
  if (!info.value) {
    try {
      await userStore.fetchInfo()
    } catch {
      // 拦截器已提示
    }
  }
  resetProfileForm()
})
</script>

<style scoped>
.avatar-block {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding-bottom: 16px;
  margin-bottom: 8px;
  border-bottom: 1px solid #f0f0f0;
}

.avatar-btn {
  margin-top: 4px;
}

.hint {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}

.muted {
  color: rgba(0, 0, 0, 0.25);
}
</style>
