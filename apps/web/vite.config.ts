import { fileURLToPath, URL } from 'node:url'
import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { AntDesignVueResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  // 后端地址，默认本地 Gin 服务
  const apiTarget = env.VITE_API_TARGET || 'http://127.0.0.1:8080'

  return {
    plugins: [
      vue(),
      Components({
        // 按需自动引入 ant-design-vue 组件，无需全量注册
        resolvers: [AntDesignVueResolver({ importStyle: false })],
        dts: 'src/components.d.ts',
      }),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 5173,
      proxy: {
        // 开发期把 /api 代理到后端，避免浏览器 CORS
        '/api': {
          target: apiTarget,
          changeOrigin: true,
        },
      },
    },
    build: {
      outDir: 'dist',
      sourcemap: mode !== 'production',
      // antd 单独成 chunk 后约 875KB（gzip 270KB），可长期缓存，无需为此告警
      chunkSizeWarningLimit: 1000,
      rollupOptions: {
        output: {
          // 把体积大且更新频率低的依赖拆出去，利用浏览器长期缓存
          manualChunks: {
            vue: ['vue', 'vue-router', 'pinia'],
            antd: ['ant-design-vue', '@ant-design/icons-vue'],
          },
        },
      },
    },
  }
})
